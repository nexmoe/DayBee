import assert from "node:assert/strict";
import { createHash, createPublicKey, verify } from "node:crypto";
import { readFileSync } from "node:fs";
import { basename, join } from "node:path";

export function verifyUpdate(directory, target) {
  const manifest=JSON.parse(readFileSync(join(directory,`update-${target}.json`),"utf8"));
  const name=decodeURIComponent(new URL(manifest.url).pathname.split("/").pop());
  assert.equal(name,basename(name),"Invalid update filename");
  const archive=readFileSync(join(directory,name));
  assert(manifest.version && manifest.signature,"Unsigned update manifest");
  assert.equal(archive.length,manifest.size,"Update archive size mismatch");
  const publicKey=process.env.DAYBEE_UPDATER_PUBLIC_KEY || readFileSync(new URL("../build/mygo-update.pub",import.meta.url),"utf8");
  const raw=Buffer.from(publicKey.trim(),"base64");
  assert.equal(raw.length,32,"Invalid Ed25519 public key");
  const key=createPublicKey({key:Buffer.concat([Buffer.from("302a300506032b6570032100","hex"),raw]),format:"der",type:"spki"});
  const digest=createHash("sha256").update(archive).digest(),signature=Buffer.from(manifest.signature,"base64");
  assert(verify(null,digest,key,signature),"Update signature mismatch");
  digest[0]^=1;
  assert(!verify(null,digest,key,signature),"Corrupted update digest accepted");
  return manifest;
}
