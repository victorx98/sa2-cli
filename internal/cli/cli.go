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
	"github.com/victorx98/sa2-cli/internal/session"
)

var errArgument = errors.New("invalid command argument")

type App struct {
	In          *os.File
	Out         io.Writer
	Err         *os.File
	Version     string
	Origin      string
	API         api.Client
	OpenSession func(context.Context) (session.Vault, *session.Locker, error)
}

type options struct {
	command   string
	origin    string
	email     string
	fromStdin bool
	help      bool
	version   bool
}

const helpText = `sa2 [--api-url HTTPS_ORIGIN] <command>

  login --email EMAIL [--password-stdin]  Sign in using an email and password
  whoami                                Refresh the session and show the identity
  logout                                Clear the local session for this origin

  --api-url HTTPS_ORIGIN                Defaults to SA2_API_URL
  --help                                Show help
  --version                             Show the CLI version
`

func (a App) Run(ctx context.Context, args []string) int {
	opts, err := parse(args, a.Origin)
	if err != nil {
		return a.failure(err)
	}
	if opts.help {
		fmt.Fprint(a.Out, helpText)
		return 0
	}
	if opts.version {
		fmt.Fprintln(a.Out, a.Version)
		return 0
	}

	origin, err := api.Origin(opts.origin)
	if err != nil {
		return a.failure(err)
	}
	vault, locks, err := a.OpenSession(ctx)
	if err != nil {
		return a.failure(err)
	}
	defer vault.Close()
	service := auth.Service{API: a.API, Vault: vault, Locks: locks}

	switch opts.command {
	case "login":
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

func parse(args []string, defaultOrigin string) (options, error) {
	opts := options{origin: defaultOrigin}
	root := flag.NewFlagSet("sa2", flag.ContinueOnError)
	root.SetOutput(io.Discard)
	root.StringVar(&opts.origin, "api-url", defaultOrigin, "")
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
	case "login", "whoami", "logout":
	default:
		return opts, errArgument
	}

	command := flag.NewFlagSet(opts.command, flag.ContinueOnError)
	command.SetOutput(io.Discard)
	command.StringVar(&opts.origin, "api-url", opts.origin, "")
	if opts.command == "login" {
		command.StringVar(&opts.email, "email", "", "")
		command.BoolVar(&opts.fromStdin, "password-stdin", false, "")
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
	detail, exit := describeError(err)
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
	case errors.Is(err, api.ErrInvalidOrigin):
		return errorDetail{"INVALID_ARGUMENT", "Set --api-url or SA2_API_URL to a valid HTTPS origin."}, 2
	case errors.Is(err, errPasswordMode):
		return errorDetail{"INVALID_ARGUMENT", "Use --password-stdin with non-terminal input, or enter the password in a terminal."}, 2
	case errors.Is(err, errPassword):
		return errorDetail{"INVALID_ARGUMENT", "Provide a nonempty password of at most 65536 bytes."}, 2
	case errors.Is(err, api.ErrInvalidRequest):
		return errorDetail{"INVALID_ARGUMENT", "SA2 rejected the supplied parameters."}, 2
	case errors.Is(err, errArgument):
		return errorDetail{"INVALID_ARGUMENT", "Check the command and options with --help."}, 2
	case errors.Is(err, auth.ErrLoginRequired):
		return errorDetail{"LOGIN_REQUIRED", "Run sa2 login to start a new session."}, 3
	case errors.Is(err, api.ErrRejected):
		return errorDetail{"AUTH_REJECTED", "SA2 rejected authentication."}, 3
	case errors.Is(err, session.ErrUnavailable):
		return errorDetail{"CREDENTIAL_STORE_UNAVAILABLE", "The operating system credential store could not complete the operation."}, 1
	case errors.Is(err, session.ErrUnsafeLock):
		return errorDetail{"SESSION_STORAGE_ERROR", "Could not access the protected session lock."}, 1
	case errors.Is(err, session.ErrBusy):
		return errorDetail{"SESSION_BUSY", "Another command is using the session. Wait for it to finish."}, 1
	case errors.Is(err, api.ErrInvalidResponse):
		return errorDetail{"INVALID_API_RESPONSE", "SA2 returned an invalid authentication response."}, 1
	case errors.Is(err, api.ErrUnavailable):
		return errorDetail{"API_UNAVAILABLE", "The SA2 API could not complete the request."}, 1
	case errors.Is(err, context.Canceled):
		return errorDetail{"CANCELLED", "The command was cancelled."}, 130
	default:
		return errorDetail{"INTERNAL_ERROR", "The command could not be completed."}, 1
	}
}
