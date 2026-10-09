# nvc — Novita Connect

Run Claude Code and Codex on Novita models with one command. nvc never edits your agent config:
everything is injected into the agent process only, and is gone when it exits.

```sh
curl -fsSL https://github.com/Alex-yang00/nvc/releases/latest/download/install.sh | sh

nvc login          # paste your Novita API key (or export NOVITA_API_KEY)
nvc claude
nvc codex
```

Installs to `~/.local/bin` (override with `NVC_INSTALL_DIR`), verifies SHA256SUMS. Pin a version with `NVC_VERSION=0.2.1`. Upgrade = run the same command again.

## Commands

Run `nvc` with no arguments in a terminal for an interactive menu (Claude Code / Codex / Login / Quit).
Set `NVC_NO_ANIMATION=1` or `NO_COLOR=1` for a static version.

```
nvc                               interactive menu
nvc login                         save your Novita API key to ~/.config/nvc/config.json (0600)
nvc claude   [--model M] [args…]  Claude Code
nvc codex    [--model M] [args…]  Codex
nvc models                        live model list with prices
nvc doctor                        check key, connectivity, installed agents
nvc uninstall [--yes]             remove nvc and its saved key (asks first)
nvc <agent> --print-env           show what would be injected (key masked), don't launch
```

`--model` / `--print-env` go right after the agent name; everything else is passed to the agent unchanged.

## Default lineup

| Claude Code tier | Model |
|---|---|
| Opus (main) | `moonshotai/kimi-k3` |
| Sonnet | `zai-org/glm-5.3` |
| Haiku (background tasks) | `deepseek/deepseek-v4.1-flash` |

Codex defaults to `zai-org/glm-5.3`. `nvc claude --model <id>` sets the main model; `/model` inside
Claude Code switches between the three tiers.

## Your config stays yours

- **Claude Code**: settings go in via `--settings` (outranks `~/.claude/settings.json`, which is not
  modified). The key is read by `apiKeyHelper` from the process env, so it never appears in `ps`.
- **Codex**: provider goes in via `-c` overrides; `~/.codex/config.toml` is not modified.
- Both agents save `/model` picks to your user config on their own. nvc snapshots those keys
  (`model` in `~/.claude/settings.json`; `model`, `model_reasoning_effort` in `~/.codex/config.toml`)
  and puts them back when the agent exits, so plain `claude` / `codex` keep working as before.
  Nothing else in those files is touched.

## Uninstall

```sh
nvc uninstall
```

Removes only what nvc added: the `nvc` binary and `~/.config/nvc/` (saved key, model cache).
Your `~/.claude/settings.json`, `~/.codex/config.toml`, shell profile and session history are not
touched — nvc never wrote anything into them that would need undoing. If the binary is already gone:
`rm -rf ~/.config/nvc`.

Codex conversations started through nvc can't be continued with plain `codex resume` afterwards
(Codex starts a new conversation; the history files stay).

## Build

```sh
scripts/build.sh 0.2.1      # vet + test + 4 platform tarballs + SHA256SUMS in dist/
scripts/smoke/run.sh claude # headless end-to-end task (uses your NOVITA_API_KEY)
scripts/smoke/run.sh codex
```

## Known limitations

- **Web search is disabled** in both agents. It's an Anthropic/OpenAI server-side tool; Novita doesn't
  run it and the model would silently make up results. Claude Code falls back to WebFetch.
- **Codex `/model` lists OpenAI models.** Picking one inside an nvc session fails against Novita.
  Use `nvc codex --model <id>` instead.
- Codex prints `Model metadata for … not found`; it uses fallback metadata and works.
- Codex's own sandbox needs unprivileged user namespaces. On Ubuntu 24.04 the default AppArmor policy
  blocks them — this affects Codex with or without nvc.
- If nvc itself is killed with `SIGKILL` mid-session, the `/model` restore above can't run.
- macOS and Linux only.

## License

MIT
