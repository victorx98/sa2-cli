import { ApiError, AuthClient } from "./api/client.js";
import { loginRequired } from "./errors.js";
import { SessionStore } from "./session-store.js";

export { apiOrigin } from "./api/client.js";
export { CliError } from "./errors.js";

export function createAuth(origin: string) {
  const client = new AuthClient(origin);
  const store = new SessionStore(origin);

  return {
    login: (email: string, password: string) =>
      store.locked(async () => {
        const { session, user } = await client.login({ email, password });
        await store.write(session);
        return user;
      }),

    whoami: () =>
      store.locked(async () => {
        let session = await store.read();
        if (!session) throw loginRequired();

        async function refresh(refreshToken: string) {
          try {
            const result = await client.refresh({ refreshToken });
            await store.write(result.session);
            return result.session;
          } catch (error) {
            if (error instanceof ApiError && error.status === 401) {
              await store.clear();
              throw loginRequired();
            }
            throw error;
          }
        }

        if (session.expiresAt - Date.now() > 30_000) {
          try {
            return await client.whoami(session.accessToken);
          } catch (error) {
            if (!(error instanceof ApiError) || !error.expired) throw error;
          }
        }

        session = await refresh(session.refreshToken);
        return client.whoami(session.accessToken);
      }),

    logout: () =>
      store.locked(async () => {
        await store.clear();
        return { loggedOut: true };
      }),
  };
}
