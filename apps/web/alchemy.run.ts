import { fileURLToPath } from "node:url";
import * as Alchemy from "alchemy";
import * as Cloudflare from "alchemy/Cloudflare";
import * as Effect from "effect/Effect";

export default Alchemy.Stack(
	"daybee-web",
	{
		providers: Cloudflare.providers(),
		state: Cloudflare.state(),
	},
	Effect.gen(function* () {
		const stage = yield* Alchemy.Stage;
		const website = yield* Cloudflare.Worker("Website", {
			name: stage === "production" ? "daybee-web" : undefined,
			domain: stage === "production" ? "daybee.co" : undefined,
			workersDev: { enabled: true, previewsEnabled: false },
			compatibility: { date: "2026-10-18" },
			assets: {
				directory: fileURLToPath(new URL("./dist", import.meta.url)),
				htmlHandling: "auto-trailing-slash",
				notFoundHandling: "404-page",
			},
		});
		return { url: website.url };
	}),
);
