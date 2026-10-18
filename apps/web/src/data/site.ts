export const site = {
	name: "DayBee",
	title: "DayBee — Local Day Recorder with Live Transcription",
	description:
		"Tray-first desktop recorder for macOS and Windows. Record long sessions, keep only speech, and transcribe locally with sherpa-onnx ASR models.",
	locale: "en_US",
	lang: "en",
	themeColor: "#ffffff",
	themeColorDark: "#0a0a0a",
	robots:
		"index, follow, max-image-preview:large, max-snippet:-1, max-video-preview:-1",
	image: "/og.jpg",
	imageAlt: "DayBee, a local desktop recorder with live transcription",
	imageWidth: 1200,
	imageHeight: 630,
	github: "https://github.com/nexmoe/eve",
	releases: "https://github.com/nexmoe/eve/releases",
} as const;

export const faqs = [
	{
		question: "What is DayBee?",
		answer:
			"A tray-first desktop recorder. It records your microphone for long sessions, splits files automatically, and transcribes speech on your computer.",
	},
	{
		question: "Which platforms are supported?",
		answer:
			"macOS 13.3+ on Apple Silicon and Intel, and Windows through an NSIS installer. System audio capture and microphone app detection are macOS features.",
	},
	{
		question: "Does my audio leave my computer?",
		answer:
			"No. Recording, voice detection, and transcription run locally with sherpa-onnx and ONNX Runtime. Only model files are downloaded the first time they are needed.",
	},
	{
		question: "Which speech models can I use?",
		answer:
			"Qwen3 ASR 0.6B is the default. FireRedASR2 AED/CTC and SenseVoice Small (2025) are also available. Switch models in Transcribe settings while recording is idle.",
	},
	{
		question: "Can it record without transcribing?",
		answer:
			"Yes. Turn off live transcription to save only detected speech audio. You can batch-transcribe existing WAV and FLAC recordings later.",
	},
	{
		question: "Where are recordings stored?",
		answer:
			"Under recordings/YYYYMMDD/ in the DayBee data folder. Each segment is a WAV or FLAC file with a matching JSON transcript and metadata.",
	},
	{
		question: "How does it stay up to date?",
		answer:
			"The packaged app checks for signed updates on startup and every six hours, then installs and restarts when recording and transcription are idle.",
	},
] as const;

export const indexablePaths = ["/"] as const;

export function absoluteUrl(origin: URL, path: string) {
	return new URL(path, origin).href;
}

export function homeJsonLd(origin: URL) {
	const url = absoluteUrl(origin, "/");
	const image = absoluteUrl(origin, site.image);
	const logo = absoluteUrl(origin, "/icon-512.png");
	return {
		"@context": "https://schema.org",
		"@graph": [
			{
				"@type": "Organization",
				"@id": `${url}#organization`,
				name: site.name,
				url,
				logo: { "@type": "ImageObject", url: logo },
				sameAs: [site.github],
			},
			{
				"@type": "WebSite",
				"@id": `${url}#website`,
				name: site.name,
				url,
				description: site.description,
				inLanguage: site.lang,
				publisher: { "@id": `${url}#organization` },
			},
			{
				"@type": "SoftwareApplication",
				"@id": `${url}#software`,
				name: site.name,
				applicationCategory: "MultimediaApplication",
				operatingSystem: "macOS 13.3+, Windows",
				url,
				downloadUrl: site.releases,
				description: site.description,
				image,
				isAccessibleForFree: true,
				offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
				featureList: [
					"Long-running microphone recording with automatic segmentation",
					"Local live transcription with sherpa-onnx",
					"VAD-only recording",
					"Optional macOS system audio capture",
					"Voiceprint labelling",
					"App-aware transcript annotations",
				],
				publisher: { "@id": `${url}#organization` },
			},
			{
				"@type": ["WebPage", "FAQPage"],
				"@id": `${url}#webpage`,
				url,
				name: site.title,
				description: site.description,
				inLanguage: site.lang,
				isPartOf: { "@id": `${url}#website` },
				about: { "@id": `${url}#software` },
				primaryImageOfPage: {
					"@type": "ImageObject",
					url: image,
					width: site.imageWidth,
					height: site.imageHeight,
				},
				mainEntity: faqs.map((item) => ({
					"@type": "Question",
					name: item.question,
					acceptedAnswer: { "@type": "Answer", text: item.answer },
				})),
			},
		],
	};
}

export function llmsText(origin: URL) {
	const home = absoluteUrl(origin, "/");
	const lines = [
		`# ${site.name}`,
		"",
		`> ${site.description}`,
		"",
		"DayBee is a tray-first desktop recorder built with Go and mygo. It records microphone audio for long sessions, segments files automatically, and uses selectable sherpa-onnx ASR models plus Silero VAD for live transcription and offline reprocessing. Everything runs on the user's computer.",
		"",
		"## Start",
		"",
		`1. Download the installer from ${site.releases}.`,
		"2. Open DayBee from the menu bar or system tray and grant microphone access.",
		"3. Start recording. Transcripts appear on the Home timeline as you speak.",
		"",
		"## Speech models",
		"",
		"- Qwen3 ASR 0.6B (default)",
		"- FireRedASR2 AED/CTC",
		"- SenseVoice Small (2025)",
		"",
		"## Links",
		"",
		`- [Website](${home})`,
		`- [Source](${site.github})`,
		`- [Releases](${site.releases})`,
		`- [Changelog](${site.github}/blob/main/CHANGELOG.md)`,
		"",
	];
	return lines.join("\n");
}
