import assert from "node:assert/strict";
import { test } from "node:test";
import { notarizeImage } from "./notarize.mjs";

function fixture({ status = "Accepted", failures = 0, errorCode = 68, validationFailures = 0 } = {}) {
  const calls = [], delays = [];
  let attempts = 0, validationAttempts = 0;
  return { calls, delays, options: {
    run(command, args) {
      assert.equal(command, "xcrun"); calls.push(args);
      if (args[0] === "notarytool") return JSON.stringify({ status, id: "acceptance-fixture" });
      if (args[1] === "staple" && attempts++ < failures) {
        throw Object.assign(new Error("The staple and validate action failed! Error " + errorCode), { status: errorCode });
      }
      if (args[1] === "validate" && validationAttempts++ < validationFailures) {
        throw Object.assign(new Error("The validate action failed! Error 68"), { status: 68 });
      }
      return "";
    },
    wait: async ms => { delays.push(ms); }, warn: () => {}
  } };
}

test("retries temporary Apple ticket lookup failures without resubmitting the signed image", async () => {
  const f = fixture({ failures: 1 });
  await notarizeImage("app.dmg", "profile", f.options);
  assert.equal(f.calls.filter(args => args[0] === "notarytool").length, 1);
  assert.equal(f.calls.filter(args => args[1] === "staple").length, 2);
  assert.deepEqual(f.delays, [5000]);
  assert.deepEqual(f.calls.at(-1), ["stapler", "validate", "app.dmg"]);
});

test("rejected notarization never staples or validates", async () => {
  const f = fixture({ status: "Invalid" });
  await assert.rejects(notarizeImage("app.dmg", "profile", f.options), /Notarization rejected/);
  assert.equal(f.calls.length, 1);
});

test("permanent stapling errors remain fatal", async () => {
  const f = fixture({ failures: 10, errorCode: 64 });
  await assert.rejects(notarizeImage("app.dmg", "profile", f.options), /Error 64/);
  assert.equal(f.calls.length, 2);
  assert.deepEqual(f.delays, []);
});

test("ticket lookup retries are bounded and validation never runs after exhaustion", async () => {
  const f = fixture({ failures: 10 });
  await assert.rejects(notarizeImage("app.dmg", "profile", f.options), /Error 68/);
  assert.equal(f.calls.filter(args => args[1] === "staple").length, 4);
  assert.deepEqual(f.delays, [5000, 10000, 20000]);
  assert.equal(f.calls.some(args => args[1] === "validate"), false);
});

test("ticket validation failures remain fatal after successful stapling", async () => {
  for (const status of [64, 65, undefined]) {
    const f = fixture();
    const run = f.options.run;
    f.options.run = (command, args) => {
      if (args[1] === "validate") throw Object.assign(new Error("Invalid stapled ticket"), { status });
      return run(command, args);
    };
    await assert.rejects(notarizeImage("app.dmg", "profile", f.options), /Invalid stapled ticket/);
    assert.deepEqual(f.delays, []);
  }
});

test("retries transient ticket validation network failures without resubmitting or restapling", async () => {
  const f = fixture({ validationFailures: 1 });
  await notarizeImage("app.dmg", "profile", f.options);
  assert.equal(f.calls.filter(args => args[0] === "notarytool").length, 1);
  assert.equal(f.calls.filter(args => args[1] === "staple").length, 1);
  assert.equal(f.calls.filter(args => args[1] === "validate").length, 2);
  assert.deepEqual(f.delays, [5000]);
});

test("persistent ticket validation network failures still block release", async () => {
  const f = fixture({ validationFailures: 10 });
  await assert.rejects(notarizeImage("app.dmg", "profile", f.options), /Error 68/);
  assert.equal(f.calls.filter(args => args[1] === "validate").length, 4);
  assert.deepEqual(f.delays, [5000, 10000, 20000]);
});
