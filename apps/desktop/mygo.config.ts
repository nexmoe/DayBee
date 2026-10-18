import { defineConfig } from "mygo-cli";
import pkg from "./package.json";
import { readFileSync } from "node:fs";

const publicKey = process.env.DAYBEE_UPDATER_PUBLIC_KEY?.trim()
  || readFileSync(new URL("./build/mygo-update.pub", import.meta.url), "utf8").trim();
const devUrl = process.env.PORTLESS_URL ? new URL(process.env.PORTLESS_URL) : undefined;

export default defineConfig(({ command }) => ({
  name: pkg.productName,
  identifier: "build.nexmoe.daybee.desktop",
  version: pkg.version,
  icon: "resources/icon.png",
  bindings: "src/renderer/src/generated/mygo.ts",
  devUrl: process.env.PORTLESS_URL,
  // The outer desktop dev command already owns the portless route and PORT.
  devCommand: "bunx --no-install vite",
  buildCommand: "bun run build:web",
  frontendDist: "out/renderer",
  out: "dist",
  macos: {
    minimumSystemVersion: "13.3",
    signingIdentity: process.env.MYGO_MACOS_SIGNING_IDENTITY || "-",
    entitlements: "build/entitlements.mac.plist",
    infoPlist: {
      LSUIElement: true,
      ...(command === "dev" && devUrl?.protocol === "http:" ? {
        NSAppTransportSecurity: {
          NSExceptionDomains: {
            [devUrl.hostname]: { NSExceptionAllowsInsecureHTTPLoads: true }
          }
        }
      } : {}),
      NSMicrophoneUsageDescription: `${pkg.productName} needs microphone access to record audio.`,
      NSSpeechRecognitionUsageDescription: `${pkg.productName} uses on-device speech recognition to transcribe your recordings.`,
      NSAudioCaptureUsageDescription: `${pkg.productName} uses system audio in memory as a voiceprint reference, and records it only when you enable System audio recording.`
    },
    ...(process.env.MYGO_NOTARY_PROFILE ? {
      notarize: { keychainProfile: process.env.MYGO_NOTARY_PROFILE }
    } : {})
  },
  windows: {
    ...(process.env.MYGO_WINDOWS_CERTIFICATE ? { certificate: process.env.MYGO_WINDOWS_CERTIFICATE } : {}),
    ...(process.env.MYGO_WINDOWS_SIGN_COMMAND ? { signCommand: process.env.MYGO_WINDOWS_SIGN_COMMAND } : {})
  },
  ...(publicKey ? {
    updates: { publicKey, github: "nexmoe/DayBee", tagPrefix: "v", deltas: 0, changelog: "../../CHANGELOG.md" }
  } : {})
}));
