import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";
import { setTimeout as delay } from "node:timers/promises";

const root = join(import.meta.dirname, "..");
const productName = JSON.parse(readFileSync(join(root, "package.json"), "utf8")).productName;
const target = process.platform === "win32" ? "windows-amd64" : `darwin-${process.arch === "x64" ? "amd64" : "arm64"}`;
const executable = process.env.DAYBEE_TEST_EXECUTABLE || join(root, "dist", target,
  ...(process.platform === "darwin" ? [`${productName}.app`, "Contents", "MacOS", productName] : [`${productName}.exe`]));

async function verifyMCP() {
  const protocolVersion = "2026-07-28";
  const store = mkdtempSync(join(tmpdir(), "daybee-mcp-"));
  const recordings = join(store, "recordings");
  mkdirSync(recordings);
  const settingsPath = join(store, "desktop-settings.json");
  const saveSettings = outputDir => writeFileSync(settingsPath, JSON.stringify({ settings: {
    desktop: {}, recording: { outputDir }
  } }));
  saveSettings(recordings);
  const text = "项目会议🙂。\n" + "保留完整文稿。".repeat(300) + "PROJECT [v2] .* 的结论。";
  const saveTranscript = (directory, content, source = "microphone") => {
    mkdirSync(directory, { recursive: true });
    writeFileSync(join(directory, "meeting.json"), JSON.stringify({
      audio_path: join(directory, "meeting.wav"), segment_start_time: "2026-10-18T09:00:00+08:00",
      audio_source: source, recording_session_id: "session", status: "ok", text: content,
      transcript_chunks: [{ text: content, startedAt: "2026-10-18T09:00:00+08:00", endedAt: "2026-10-18T09:00:10+08:00", voiceIdentity: "me" }]
    }));
  };
  saveTranscript(recordings, text);
  writeFileSync(join(recordings, "audio-only.recording.json"), JSON.stringify({ audio_path: join(recordings, "audio-only.wav"), segment_start_time: "2026-10-18T10:00:00+08:00" }));
  const env = { ...process.env, DAYBEE_STORE_DIR: store };
  delete env.DAYBEE_VERIFY_AUDIO;
  delete env.DAYBEE_VERIFY_NATIVE;
  delete env.DAYBEE_E2E_TEST;
  delete env.DYLD_LIBRARY_PATH;
  const child = spawn(executable, ["--mcp"], { env, stdio: ["pipe", "pipe", "pipe"] });
  let logs = "", nextID = 0, protocolError;
  const pending = new Map();
  child.stderr.on("data", chunk => { logs += chunk; });
  const fail = error => { for (const request of pending.values()) request.reject(error); pending.clear(); };
  child.once("error", fail);
  child.once("exit", code => fail(new Error(`MCP exited ${code}: ${logs}`)));
  const lines = createInterface({ input: child.stdout });
  lines.on("line", line => {
    try {
      const message = JSON.parse(line);
      assert.equal(message.jsonrpc, "2.0", "stdout must contain only JSON-RPC messages");
      if (!pending.has(message.id)) return;
      const request = pending.get(message.id);
      pending.delete(message.id);
      if (message.error) request.reject(Object.assign(new Error(JSON.stringify(message.error)), message.error));
      else request.resolve(message.result);
    } catch (error) { protocolError = error; fail(error); }
  });
  const request = (method, params = {}) => {
    if (protocolError) return Promise.reject(protocolError);
    if (method !== "initialize" && !("_meta" in params)) params = { ...params, _meta: {
      "io.modelcontextprotocol/protocolVersion": protocolVersion,
      "io.modelcontextprotocol/clientCapabilities": {},
      "io.modelcontextprotocol/clientInfo": { name: "daybee-mcp-test", version: "1" }
    } };
    const id = ++nextID;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { pending.delete(id); reject(new Error(`Timeout: ${method}\n${logs}`)); }, 10000);
      pending.set(id, { resolve: result => { clearTimeout(timer); resolve(result); }, reject: error => { clearTimeout(timer); reject(error); } });
      child.stdin.write(JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n");
    });
  };
  const call = async (name, args = {}) => {
    const result = await request("tools/call", { name, arguments: args });
    assert(!result.isError, JSON.stringify(result));
    assert.equal(result.resultType, "complete");
    return result.structuredContent;
  };
  const checkCache = result => {
    assert.equal(result.ttlMs, 0, "live transcripts must be re-fetched");
    assert.equal(result.cacheScope, "private", "local transcripts must not be shared by caches");
    assert.equal(result._meta["io.modelcontextprotocol/serverInfo"].name, "daybee");
  };
  try {
    for (const version of ["2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"]) {
      await assert.rejects(request("initialize", {
        protocolVersion: version, capabilities: {}, clientInfo: { name: "legacy-test", version: "1" }
      }), error => error.code === -32022 && error.message.includes(protocolVersion)
        && error.data.supported.length === 1 && error.data.supported[0] === protocolVersion);
    }
    const discovered = await request("server/discover");
    assert.deepEqual(discovered.supportedVersions, [protocolVersion]);
    assert(discovered.capabilities.tools && discovered.capabilities.resources);
    checkCache(discovered);
    const tools = await request("tools/list");
    checkCache(tools);
    assert.deepEqual(tools.tools.map(tool => tool.name).sort(), ["list_transcripts", "read_transcript", "search_transcripts"]);
    assert(tools.tools.every(tool => tool.annotations.readOnlyHint));
    const settingsBefore = readFileSync(settingsPath, "utf8");
    const list = await call("list_transcripts", { limit: 1 });
    assert.equal(list.total, 1, "audio-only archives are not transcript documents");
    const document = await call("read_transcript", { id: list.items[0].id });
    assert.equal(document.text, text);
    assert.equal(document.chunks[0].voiceIdentity, "me");
    assert.equal(document.chunks[0].startedAt, "2026-10-18T09:00:00+08:00");
    const found = await call("search_transcripts", { query: "project [v2] .*" });
    assert.equal(found.total, 1);
    assert(found.items[0].preview.includes("PROJECT [v2] .*"));
    const resources = await request("resources/list");
    assert.equal(resources.resources[0].uri, "daybee://transcripts");
    checkCache(resources);
    const templates = await request("resources/templates/list");
    assert.equal(templates.resourceTemplates[0].uriTemplate, "daybee://transcripts/{id}");
    checkCache(templates);
    const index = await request("resources/read", { uri: "daybee://transcripts" });
    assert.equal(JSON.parse(index.contents[0].text).total, 1);
    checkCache(index);
    const content = await request("resources/read", { uri: document.uri });
    assert.equal(JSON.parse(content.contents[0].text).text, text);
    checkCache(content);
    assert.equal(readFileSync(settingsPath, "utf8"), settingsBefore);
    saveTranscript(recordings, text + "刚完成的新语音。");
    const fresh = await call("read_transcript", { id: document.id });
    assert(fresh.text.endsWith("刚完成的新语音。"), "newly completed speech requires no server restart");
    const freshResource = await request("resources/read", { uri: document.uri });
    assert(JSON.parse(freshResource.contents[0].text).text.endsWith("刚完成的新语音。"));
    checkCache(freshResource);
    const next = join(store, "new-recordings");
    saveTranscript(next, "新录音目录的文稿。", "system");
    saveSettings(next);
    const switched = await call("list_transcripts", { source: "system" });
    assert.equal(switched.total, 1);
    assert.equal((await call("read_transcript", { id: switched.items[0].id })).text, "新录音目录的文稿。");
    const rejected = await request("tools/call", { name: "read_transcript", arguments: { id: "../../desktop-settings.json" } });
    assert.equal(rejected.isError, true);
    await assert.rejects(request("tools/list", { _meta: { "io.modelcontextprotocol/clientCapabilities": {} } }), { code: -32602 });
    await assert.rejects(request("tools/list", { _meta: { "io.modelcontextprotocol/protocolVersion": protocolVersion } }), { code: -32602 });
    for (const version of ["2025-11-25", "2099-01-01"]) {
      await assert.rejects(request("tools/list", { _meta: {
        "io.modelcontextprotocol/protocolVersion": version, "io.modelcontextprotocol/clientCapabilities": {}
      } }), error => error.code === -32022 && error.data.supported.length === 1 && error.data.supported[0] === protocolVersion);
    }
    await assert.rejects(request("resources/read", { uri: "daybee://transcripts/unknown" }), { code: -32602 });
    assert.equal((await call("list_transcripts", { source: "system" })).total, 1, "protocol errors must not poison later valid requests");
    child.stdin.end();
    const exit = await Promise.race([new Promise(resolve => child.once("exit", resolve)), delay(5000, undefined, { ref: false }).then(() => { throw new Error("MCP did not exit after stdin closed"); })]);
    assert.equal(exit, 0, logs);
    console.log(`MCP ${protocolVersion} ${target}: stateless discovery, legacy rejection, private cache policy, three read-only tools, full text/resources, live updates, current output folder and clean shutdown passed.`);
  } finally {
    lines.close();
    if (child.exitCode === null) child.kill("SIGTERM");
    rmSync(store, { recursive: true, force: true });
  }
}

await verifyMCP();
