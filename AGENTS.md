1. A single file must not exceed 500 lines.
2. Commits and PRs must follow the Conventional specification. Commit messages must clearly describe the specific functionality and content of the code changes. If one line is not enough, break it into multiple bullet points on new lines.
3. Before changing a documented subsystem, read `docs/subsystem/README.md` and its page, and check the source it links. When the contract changes, update that page in the same PR. `bun run check:docs` must pass.
