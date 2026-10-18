import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { visualCases } from "./visual-fixtures.mjs";

const root=resolve(import.meta.dirname);
const target=process.env.DAYBEE_BUILD_TARGET || `darwin-${process.arch==="x64"?"amd64":"arm64"}`;
const baseline=join(root,"baseline",`electron-${target}`), actual=join(root,"results",target);
const approved=join(root,"baseline",`approved-no-ffmpeg-${target}`);
const manifest=JSON.parse(readFileSync(join(baseline,"manifest.json"),"utf8"));
const hash=path=>createHash("sha256").update(readFileSync(path)).digest("hex");
for(const [path,digest] of Object.entries(manifest.sha256)) assert.equal(hash(join(baseline,path)),digest,`Original baseline changed: ${path}`);
const exception=JSON.parse(readFileSync(join(approved,"manifest.json"),"utf8"));
assert.equal(exception.sourceCommit,manifest.sourceCommit); assert.equal(exception.approvedChange,"remove-ffmpeg-status");
for(const [path,digest] of Object.entries(exception.sha256)) assert.equal(hash(join(approved,path)),digest,`Approved baseline changed: ${path}`);
const report={target,sourceCommit:manifest.sourceCommit,approvedUIChanges:["remove-ffmpeg-status"],geometryTolerance:0.125,canvasTolerance:1e-10,visible:true,cases:[]};
const sameCanvas=(a,b)=>{
  if(!Array.isArray(a)||!Array.isArray(b)) return a===b;
  return a.length===b.length && a.every((value,i)=>Array.isArray(value)?sameCanvas(value,b[i]):
    typeof value==="number"?typeof b[i]==="number"&&Math.abs(value-b[i])<=report.canvasTolerance:value===b[i]);
};
for(const item of visualCases()) {
  const expectedRoot=item.tab===3?approved:baseline;
  const expected=JSON.parse(readFileSync(join(expectedRoot,item.name+".json"),"utf8"));
  const observed=JSON.parse(readFileSync(join(actual,item.name+".json"),"utf8"));
  const differences=[];let largestDelta=0;
  report.visible &&= !observed.hidden;
  if(JSON.stringify(expected.dimensions)!==JSON.stringify(observed.dimensions)) differences.push("Window dimensions changed");
  if(expected.dark!==observed.dark) differences.push("Theme changed");
  if(!sameCanvas(expected.canvas,observed.canvas)) differences.push("Waveform drawing changed");
  if(expected.elements.length!==observed.elements.length) differences.push("Element count changed");
  expected.elements.forEach((element,i)=>{
    const other=observed.elements[i];if(!other)return;
    for(const key of ["tag","role","text","color"]) if(element[key]!==other[key]) differences.push(`${key}: ${element.text}`);
    const delta=Math.max(...element.rect.map((value,index)=>Math.abs(value-other.rect[index])));largestDelta=Math.max(largestDelta,delta);
    if(delta>report.geometryTolerance) differences.push(`Geometry ${delta.toFixed(4)}px: ${element.text}`);
  });
  report.cases.push({name:item.name,largestDelta,identicalPNG:hash(join(expectedRoot,item.name+".png"))===hash(join(actual,item.name+".png")),differences});
}
report.passed=report.cases.every(item=>item.differences.length===0);
report.windowAcceptance=false; // Native chrome, materials and raster differences still need human review.
writeFileSync(join(actual,"visual-comparison.json"),JSON.stringify(report,null,2)+"\n");
assert(report.passed,`Visual differences remain; inspect ${join(actual,"visual-comparison.json")}`);
if(!process.argv.includes("--allow-hidden")) assert(report.visible,"Unlock the desktop and recapture visible native windows");
console.log(`${report.cases.length} fixtures: content, color, geometry and waveform match${report.visible?"":" (hidden-page diagnostics only)"}. Pixel/native-window acceptance remains separate.`);
