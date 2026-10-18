import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(import.meta.dirname, "..");
const ENFORCED_PREFIX = "docs/subsystem/";
const INLINE_LINK = /!?\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)/gu;
const DEFINITION = /^\s{0,3}\[[^\]]+\]:\s*<?(\S+?)>?(?:\s+.*)?$/u;
// Backticked paths that start at a top-level repo directory. Ellipses, globs and spaces end the match.
const SOURCE_PATH = /`((?:apps|packages|scripts|docs|\.agents|\.github)\/[^`*{}<>|\s]+)`/gu;
const REMOVAL = /删除|移除|removed|deleted/iu;

function walkMarkdown(directory, files = []) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) walkMarkdown(path, files);
    else if (entry.name.endsWith(".md")) files.push(path);
  }
  return files;
}

// Only relative targets: URLs, root paths and anchors are skipped.
export const isCheckedTarget = (target) =>
  !target.startsWith("#") && !target.startsWith("/") && !/^[a-z][a-z0-9+.-]*:/iu.test(target);

function targetPath(target) {
  const raw = target.replace(/[#?].*$/u, "");
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

function linesOutsideFences(text) {
  let inFence = false;
  return text.split("\n").map((line) => {
    if (/^\s*(```|~~~)/u.test(line)) {
      inFence = !inFence;
      return "";
    }
    return inFence ? "" : line;
  });
}

export function extractLinkTargets(text) {
  return linesOutsideFences(text).flatMap((line, index) => {
    // `[x](y)` inside inline code is not a link.
    const prose = line.replace(/`[^`]*`/gu, "");
    const targets = [...prose.matchAll(INLINE_LINK)].map((match) => match[1]);
    const definition = prose.match(DEFINITION);
    if (definition) targets.push(definition[1]);
    return targets.map((target) => ({ line: index + 1, target }));
  });
}

// Drop line numbers, fragments and directory prefixes; skip untracked secrets and build output.
export function sourceFilePath(raw) {
  const path = raw.replace(/[#?].*$/u, "").replace(/:\d.*$/u, "").replace(/:$/u, "");
  if (path.endsWith("/") || path.includes("...")) return null;
  if (path.endsWith(".env") || /\/(?:dist|out|node_modules|test-results)(?:\/|$)/u.test(path)) return null;
  return path;
}

// Lines that say a file was removed are not references to it.
export function extractSourcePaths(text) {
  return linesOutsideFences(text).flatMap((line, index) => {
    if (REMOVAL.test(line)) return [];
    return [...line.matchAll(SOURCE_PATH)]
      .map((match) => sourceFilePath(match[1]))
      .filter(Boolean)
      .map((path) => ({ line: index + 1, path }));
  });
}

export function findBrokenLinksInText(text, sourcePath, exists, rootDir = root) {
  return [
    ...extractLinkTargets(text)
      .filter(({ target }) => isCheckedTarget(target))
      .filter(({ target }) => !exists(resolve(dirname(sourcePath), targetPath(target))))
      .map(({ line, target }) => ({ kind: "link", line, target })),
    ...extractSourcePaths(text)
      .filter(({ path }) => !exists(resolve(rootDir, path)))
      .map(({ line, path }) => ({ kind: "path", line, target: path })),
  ];
}

export const isEnforced = (relativePath) => relativePath.startsWith(ENFORCED_PREFIX);

export function collectBrokenLinks(rootDir = root) {
  return walkMarkdown(join(rootDir, "docs")).flatMap((file) =>
    findBrokenLinksInText(readFileSync(file, "utf8"), file, existsSync, rootDir).map((finding) => ({
      ...finding,
      relativePath: relative(rootDir, file).split(sep).join("/"),
    })),
  );
}

// Only docs/subsystem/ fails the check. The rest of docs/ is history and is only reported.
export function report(findings, write = console) {
  const enforced = findings.filter((finding) => isEnforced(finding.relativePath));
  const reported = findings.filter((finding) => !isEnforced(finding.relativePath));
  if (enforced.length > 0) {
    write.error(`Found ${enforced.length} broken reference(s) in docs/subsystem/:`);
    for (const { relativePath, line, target } of enforced) write.error(`- ${relativePath}:${line} → ${target}`);
  } else {
    write.log("All relative links and source paths in docs/subsystem/ resolve.");
  }
  if (reported.length > 0) {
    write.error(`Additionally ${reported.length} broken reference(s) elsewhere in docs/ (not enforced):`);
    for (const { relativePath, line, target } of reported) write.error(`- ${relativePath}:${line} → ${target}`);
  }
  return enforced.length > 0 ? 1 : 0;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = report(collectBrokenLinks());
}
