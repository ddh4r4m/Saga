// Service configuration, loaded from an env-file style string.
import { parseEnvFile } from "envfile-parse-strict";

export interface Config {
  port: number;
  host: string;
  appName: string;
  debug: boolean;
  motd: string;
}

export function loadConfig(text: string): Config {
  const env = parseEnvFile(text) as Record<string, string>;
  const port = Number(env.PORT ?? "8080");
  if (!Number.isInteger(port) || port <= 0) throw new Error(`invalid PORT: ${env.PORT}`);
  return {
    port,
    host: env.HOST ?? "127.0.0.1",
    appName: env.APP_NAME ?? "order-service",
    debug: (env.DEBUG ?? "false").toLowerCase() === "true",
    motd: env.MOTD ?? "",
  };
}
