package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"

	"github.com/victorx98/sa2-cli/internal/api"
	"github.com/victorx98/sa2-cli/internal/auth"
	"github.com/victorx98/sa2-cli/internal/booking"
	"github.com/victorx98/sa2-cli/internal/config"
	"github.com/victorx98/sa2-cli/internal/session"
)

var (
	errArgument    = errors.New("invalid command argument")
	errFeishuHTTPS = errors.New("Feishu web authorization requires HTTPS")
)

type App struct {
	In          *os.File
	Out         io.Writer
	Err         *os.File
	Version     string
	LoadConfig  func() (config.Settings, error)
	API         api.Client
	OpenSession func(context.Context) (session.Vault, *session.Locker, error)
}

type options struct {
	sessions  booking.Command
	command   string
	apiURL    *string
	dev       bool
	email     string
	fromStdin bool
	feishu    bool
	help      bool
	version   bool
}

const helpText = `sa2 [--dev | --api-url ORIGIN] <command>

  login --email EMAIL [--password-stdin]  Sign in using an email and password
  login --feishu                         Scan with Feishu in a browser on this computer
  whoami                                Refresh the session and show the identity
  logout                                Clear the local session for this origin
  sessions                              Find, preview, commit and track session bookings

  --dev                                 Use local sa2-api at ` + config.DevelopmentOrigin + `
  --api-url ORIGIN                      Overrides SA2_API_URL and config.json
  --help                                Show help
  --version                             Show the CLI version

  Configuration: sa2-cli/config.json in the user configuration directory
  Default API origin: ` + config.DefaultOrigin + `
`

func (a App) Run(ctx context.Context, args []string) int {
	opts, err := parse(args)
	if err != nil {
		return a.failure(err)
	}
	if opts.dev && opts.apiURL != nil {
		return a.writeError(errorDetail{"INVALID_ARGUMENT", "Choose either --dev or --api-url."}, 2)
	}
	if opts.help && opts.command == "sessions" {
		fmt.Fprint(a.Out, booking.Help)
		return 0
	}
	if opts.help {
		fmt.Fprint(a.Out, helpText)
		return 0
	}
	if opts.version {
		fmt.Fprintln(a.Out, a.Version)
		return 0
	}

	target := config.DevelopmentOrigin
	if opts.apiURL != nil {
		target = *opts.apiURL
	} else if !opts.dev {
		settings, err := a.LoadConfig()
		if err != nil {
			return a.writeError(errorDetail{"CONFIG_ERROR", err.Error()}, 1)
		}
		target = settings.Origin
	}
	origin, err := api.Origin(target)
	if err != nil {
		return a.failure(err)
	}
	if opts.feishu && !strings.HasPrefix(origin, "https://") {
		return a.failure(errFeishuHTTPS)
	}
	vault, locks, err := a.OpenSession(ctx)
	if err != nil {
		return a.failure(err)
	}
	defer vault.Close()
	service := auth.Service{API: a.API, Vault: vault, Locks: locks}

	switch opts.command {
	case "sessions":
		result, err := service.Renew(ctx, origin)
		if err != nil {
			return a.failure(err)
		}
		defer clear(result.AccessToken)
		client := booking.Client{API: a.API, Origin: origin, Actor: result.Identity.ID, Token: result.AccessToken}
		data, err := client.Run(ctx, opts.sessions)
		return a.result(data, err)
	case "login":
		if opts.feishu {
			identity, err := service.FeishuLogin(ctx, origin, a.Err)
			return a.result(identity, err)
		}
		identity, err := service.Login(ctx, origin, opts.email, func(ctx context.Context) ([]byte, error) {
			return a.password(ctx, opts.fromStdin)
		})
		return a.result(identity, err)
	case "whoami":
		identity, err := service.Whoami(ctx, origin)
		return a.result(identity, err)
	case "logout":
		err := service.Logout(ctx, origin)
		return a.result(map[string]bool{"loggedOut": true}, err)
	default:
		return a.failure(errArgument)
	}
}

func parse(args []string) (options, error) {
	var opts options
	setAPIURL := func(value string) error {
		opts.apiURL = &value
		return nil
	}
	root := flag.NewFlagSet("sa2", flag.ContinueOnError)
	root.SetOutput(io.Discard)
	root.BoolVar(&opts.dev, "dev", false, "")
	root.Func("api-url", "", setAPIURL)
	root.BoolVar(&opts.version, "version", false, "")
	root.BoolVar(&opts.version, "v", false, "")
	if err := root.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			opts.help = true
			return opts, nil
		}
		return opts, errArgument
	}
	if root.NArg() == 0 {
		opts.help = !opts.version
		return opts, nil
	}
	if opts.version {
		return opts, errArgument
	}
	opts.command = root.Arg(0)
	switch opts.command {
	case "sessions":
		cmd, err := booking.Parse(root.Args()[1:])
		if errors.Is(err, booking.ErrHelp) {
			opts.help = true
			return opts, nil
		}
		if err != nil {
			return opts, errArgument
		}
		opts.sessions = cmd
		return opts, nil
	case "login", "whoami", "logout":
	default:
		return opts, errArgument
	}

	command := flag.NewFlagSet(opts.command, flag.ContinueOnError)
	command.SetOutput(io.Discard)
	command.Func("api-url", "", setAPIURL)
	if opts.command == "login" {
		command.StringVar(&opts.email, "email", "", "")
		command.BoolVar(&opts.fromStdin, "password-stdin", false, "")
		command.BoolVar(&opts.feishu, "feishu", false, "")
	}
	if err := command.Parse(root.Args()[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			opts.help = true
			return opts, nil
		}
		return opts, errArgument
	}
	if command.NArg() != 0 {
		return opts, errArgument
	}
	if opts.command == "login" {
		if opts.feishu {
			if opts.email != "" || opts.fromStdin {
				return opts, errArgument
			}
			return opts, nil
		}
		opts.email = strings.TrimSpace(opts.email)
		address, err := mail.ParseAddress(opts.email)
		if err != nil || address.Address != opts.email {
			return opts, errArgument
		}
	}
	return opts, nil
}

func (a App) result(data any, err error) int {
	if err != nil {
		return a.failure(err)
	}
	if err := json.NewEncoder(a.Out).Encode(struct {
		Data any `json:"data"`
	}{Data: data}); err != nil {
		return 1
	}
	return 0
}

func (a App) failure(err error) int {
	var problem *booking.Failure
	if errors.As(err, &problem) {
		_ = json.NewEncoder(a.Out).Encode(map[string]any{"error": problem})
		return 1
	}
	var business *api.BusinessError
	if errors.As(err, &business) {
		return a.writeError(errorDetail{business.Code, "SA2 rejected the request."}, 1)
	}
	if errors.Is(err, session.ErrPlanExists) {
		return a.writeError(errorDetail{"PLAN_FILE_EXISTS", "Choose a new --out path for this preview."}, 2)
	}
	if errors.Is(err, session.ErrPlanFile) {
		return a.writeError(errorDetail{"INVALID_PLAN_FILE", "Use a private plan for this account, origin and action."}, 2)
	}

	detail, exit := describeError(err)
	return a.writeError(detail, exit)
}

func (a App) writeError(detail errorDetail, exit int) int {
	_ = json.NewEncoder(a.Out).Encode(struct {
		Error errorDetail `json:"error"`
	}{Error: detail})
	return exit
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func describeError(err error) (errorDetail, int) {
	switch {
	case errors.Is(err, api.ErrBookingAPINotDeployed):
		return errorDetail{"BOOKING_API_NOT_DEPLOYED", err.Error()}, 1
	case errors.Is(err, api.ErrInvalidOrigin):
		return errorDetail{"INVALID_ARGUMENT", "Use an HTTPS origin or a loopback HTTP origin in --api-url, SA2_API_URL, or config.json."}, 2
	case errors.Is(err, errPasswordMode):
		return errorDetail{"INVALID_ARGUMENT", "Use --password-stdin with non-terminal input, or enter the password in a terminal."}, 2
	case errors.Is(err, errPassword):
		return errorDetail{"INVALID_ARGUMENT", "Provide a nonempty password of at most 65536 bytes."}, 2
	case errors.Is(err, errFeishuHTTPS):
		return errorDetail{"INVALID_ARGUMENT", "Use an HTTPS API origin for Feishu login."}, 2
	case errors.Is(err, api.ErrInvalidRequest):
		return errorDetail{"INVALID_ARGUMENT", "SA2 rejected the supplied parameters."}, 2
	case errors.Is(err, errArgument):
		return errorDetail{"INVALID_ARGUMENT", "Check the command and options with --help."}, 2
	case errors.Is(err, auth.ErrLoginRequired):
		return errorDetail{"LOGIN_REQUIRED", "Run sa2 login to start a new session."}, 3
	case errors.Is(err, api.ErrRejected):
		return errorDetail{"AUTH_REJECTED", "SA2 rejected authentication."}, 3
	case errors.Is(err, context.DeadlineExceeded):
		return errorDetail{"AUTH_TIMEOUT", "Feishu authorization timed out. Run sa2 login --feishu to try again."}, 1
	case errors.Is(err, session.ErrUnavailable):
		return errorDetail{"CREDENTIAL_STORE_UNAVAILABLE", "The operating system credential store could not complete the operation."}, 1
	case errors.Is(err, session.ErrUnsafeLock):
		return errorDetail{"SESSION_STORAGE_ERROR", "Could not access the protected session lock."}, 1
	case errors.Is(err, session.ErrBusy):
		return errorDetail{"SESSION_BUSY", "Another command is using the session. Wait for it to finish."}, 1
	case errors.Is(err, api.ErrInvalidResponse):
		return errorDetail{"INVALID_API_RESPONSE", "SA2 returned an invalid authentication response."}, 1
	case errors.Is(err, auth.ErrFeishuCallbackUnavailable):
		return errorDetail{"CALLBACK_UNAVAILABLE", "Could not start the local Feishu browser callback."}, 1
	case errors.Is(err, api.ErrUnavailable):
		return errorDetail{"API_UNAVAILABLE", "The SA2 API could not complete the request."}, 1
	case errors.Is(err, context.Canceled):
		return errorDetail{"CANCELLED", "The command was cancelled."}, 130
	default:
		return errorDetail{"INTERNAL_ERROR", "The command could not be completed."}, 1
	}
}
