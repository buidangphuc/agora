import { spawnSync } from "node:child_process";
import { isMain, root } from "./lib.mjs";

const LOOPBACK = "//127.0.0.1:4873/";
const TOKEN_KEY = `npm_config_${LOOPBACK}:_authToken`;

export function publishEnv(env = process.env) {
  if (env[TOKEN_KEY]) return env;
  return { ...env, [TOKEN_KEY]: "dev-local-anonymous" };
}

if (isMain(import.meta.url)) {
  const run = spawnSync("npx", ["changeset", "publish", "--no-git-tag"], {
    cwd: root,
    stdio: "inherit",
    env: publishEnv(),
  });
  process.exit(run.status ?? 1);
}
