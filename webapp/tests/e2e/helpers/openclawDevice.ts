import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";

function b64UrlToB64(s: string): string {
  const v = String(s ?? "").trim().replace(/-/g, "+").replace(/_/g, "/");
  const pad = v.length % 4 === 0 ? "" : "=".repeat(4 - (v.length % 4));
  return v + pad;
}

export function requireOpenclawDevicePublicKey(): string {
  const p = path.join(os.homedir(), ".openclaw", "identity", "device.json");
  const raw = fs.readFileSync(p, "utf8");
  const j = JSON.parse(raw) as { publicKeyPem?: string };
  const pubPem = String(j?.publicKeyPem ?? "").trim();
  if (!pubPem) throw new Error(`OpenClaw device publicKeyPem missing: ${p}`);

  const pubKey = crypto.createPublicKey(pubPem);
  const jwk = pubKey.export({ format: "jwk" }) as { x?: string };
  if (!jwk?.x) throw new Error("OpenClaw device JWK export missing x.");
  return `ed25519:${b64UrlToB64(jwk.x)}`;
}

