// Validates every examples/<service>/<kind>.yaml against its CRD schema.
// Optional fields that violate a constraint are pruned in place (with the
// header comment preserved); required-field violations are reported as errors.
//
// Run: NODE_PATH=<koncierge>/node_modules node hack/validate-examples.mjs [--fix]
import { readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { createRequire } from "node:module";

const require = createRequire(process.env.NODE_PATH + "/");
const { Ajv } = require("ajv");
const YAML = require("yaml");

const ROOT = new URL("..", import.meta.url).pathname;
const fix = process.argv.includes("--fix");

// Load CRD schemas by kind
const schemas = new Map();
for (const f of readdirSync(join(ROOT, "helm/konfig-konector/crds"))) {
  const doc = YAML.parse(readFileSync(join(ROOT, "helm/konfig-konector/crds", f), "utf8"));
  if (doc?.kind !== "CustomResourceDefinition") continue;
  schemas.set(doc.spec.names.kind, doc.spec.versions[0].schema.openAPIV3Schema);
}

const ajv = new Ajv({ strict: false, allErrors: true, validateFormats: false });
const compiled = new Map();

function isRequiredPath(schema, segs) {
  // walk schema along segs; report whether the leaf is required at its parent
  let node = schema;
  let required = true;
  for (let i = 0; i < segs.length; i++) {
    const seg = segs[i];
    if (node.type === "array") { node = node.items ?? {}; continue; }
    if (/^\d+$/.test(seg)) { node = node.items ?? node; continue; }
    const req = new Set(node.required ?? []);
    required = req.has(seg);
    node = node.properties?.[seg] ?? {};
  }
  return required;
}

function deletePath(obj, segs) {
  let node = obj;
  for (let i = 0; i < segs.length - 1; i++) {
    const seg = /^\d+$/.test(segs[i]) ? Number(segs[i]) : segs[i];
    node = node?.[seg];
    if (node == null) return false;
  }
  const last = /^\d+$/.test(segs.at(-1)) ? Number(segs.at(-1)) : segs.at(-1);
  if (Array.isArray(node)) node.splice(last, 1);
  else if (node && typeof node === "object") delete node[last];
  else return false;
  return true;
}

let files = 0, ok = 0, prunedFiles = 0, hardErrors = [];
for (const svc of readdirSync(join(ROOT, "examples"))) {
  const dir = join(ROOT, "examples", svc);
  if (!statSync(dir).isDirectory()) continue;
  for (const f of readdirSync(dir)) {
    if (!f.endsWith(".yaml")) continue;
    files++;
    const path = join(dir, f);
    const raw = readFileSync(path, "utf8");
    const header = raw.split("\n").filter((l) => l.startsWith("#")).join("\n");
    const doc = YAML.parse(raw);
    const schema = schemas.get(doc?.kind);
    if (!schema) { hardErrors.push(`${svc}/${f}: unknown kind ${doc?.kind}`); continue; }

    let fn = compiled.get(doc.kind);
    if (!fn) { fn = ajv.compile(schema); compiled.set(doc.kind, fn); }

    let pruned = false;
    for (let round = 0; round < 25; round++) {
      if (fn(doc)) break;
      const err = (fn.errors ?? [])[0];
      if (!err) break;
      const segs = err.instancePath.split("/").filter(Boolean);
      // a "required" error points at the parent; the missing field is truly required
      if (err.keyword === "required") {
        hardErrors.push(`${svc}/${f}: missing required ${err.instancePath}/${err.params.missingProperty}`);
        break;
      }
      if (segs.length === 0 || isRequiredPath(schema, segs)) {
        hardErrors.push(`${svc}/${f}: required-field violation ${err.instancePath} ${err.message}`);
        break;
      }
      if (!fix) { hardErrors.push(`${svc}/${f}: ${err.instancePath} ${err.message}`); break; }
      if (!deletePath(doc, segs)) {
        hardErrors.push(`${svc}/${f}: could not prune ${err.instancePath}`);
        break;
      }
      pruned = true;
    }
    if (fn(doc)) {
      ok++;
      if (pruned && fix) {
        prunedFiles++;
        writeFileSync(path, header + "\n" + YAML.stringify(doc, { lineWidth: 100 }));
      }
    }
  }
}

console.log(`validated ${files} examples: ${ok} valid, ${prunedFiles} auto-pruned, ${hardErrors.length} errors`);
for (const e of hardErrors.slice(0, 25)) console.log("  ERROR " + e);
process.exit(hardErrors.length ? 1 : 0);
