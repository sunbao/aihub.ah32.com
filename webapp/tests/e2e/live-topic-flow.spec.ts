import { expect, test } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import crypto from "node:crypto";
import { isLiveMode, requireEnv } from "./helpers/liveAuth";

function b64UrlToB64(s: string): string {
  const v = String(s ?? "").trim().replace(/-/g, "+").replace(/_/g, "/");
  const pad = v.length % 4 === 0 ? "" : "=".repeat(4 - (v.length % 4));
  return v + pad;
}

async function adminDeleteAgent(
  request: APIRequestContext,
  baseURL: string,
  adminApiKey: string,
  agentRef: string,
): Promise<void> {
  const ref = String(agentRef ?? "").trim();
  if (!ref) return;
  const res = await request.delete(`${baseURL}/v1/agents/${encodeURIComponent(ref)}`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!res.ok()) throw new Error(`Delete agent failed, status=${res.status()}`);
}

async function adminDeleteTopic(
  request: APIRequestContext,
  baseURL: string,
  adminApiKey: string,
  topicId: string,
): Promise<void> {
  const tid = String(topicId ?? "").trim();
  if (!tid) return;
  const res = await request.delete(`${baseURL}/v1/admin/oss/topics/${encodeURIComponent(tid)}`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!res.ok()) throw new Error(`Delete topic failed, status=${res.status()}`);
}

async function createAdmittedAgent(
  request: APIRequestContext,
  baseURL: string,
  adminApiKey: string,
  name: string,
): Promise<{ agentRef: string; agentKey: string }> {
  const { publicKey, privateKey } = crypto.generateKeyPairSync("ed25519");
  const pubJwk = publicKey.export({ format: "jwk" }) as { x?: string };
  if (!pubJwk?.x) throw new Error("Failed to export Ed25519 public key as JWK.");
  const pubB64 = b64UrlToB64(pubJwk.x);

  const create = await request.post(`${baseURL}/v1/agents`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
    data: {
      name,
      description: "e2e: topic participation + evaluation (oss credentials)",
      tags: ["e2e", "topic-flow"],
      agent_public_key: pubB64,
    },
  });
  if (!create.ok()) throw new Error(`Create agent failed, status=${create.status()}`);
  const cj = (await create.json()) as { agent_ref?: string; api_key?: string };
  const agentRef = String(cj.agent_ref ?? "").trim();
  const agentKey = String(cj.api_key ?? "").trim();
  if (!agentRef || !agentKey) throw new Error("Create agent response missing agent_ref/api_key.");

  const start = await request.post(`${baseURL}/v1/agents/${encodeURIComponent(agentRef)}/admission/start`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
  });
  if (!start.ok()) throw new Error(`Admission start failed, status=${start.status()}`);
  const sj = (await start.json()) as { challenge?: string };
  const challenge = String(sj.challenge ?? "").trim();
  if (!challenge) throw new Error("Admission start response missing challenge.");

  const sig = crypto.sign(null, Buffer.from(challenge, "utf-8"), privateKey);
  const signatureB64 = sig.toString("base64");

  const complete = await request.post(`${baseURL}/v1/agents/${encodeURIComponent(agentRef)}/admission/complete`, {
    headers: { Authorization: `Bearer ${agentKey}` },
    data: { signature: signatureB64 },
  });
  if (!complete.ok()) throw new Error(`Admission complete failed, status=${complete.status()}`);

  return { agentRef, agentKey };
}

async function adminCreatePoetryDuelTopic(
  request: APIRequestContext,
  baseURL: string,
  adminApiKey: string,
  allowAgentRef: string,
): Promise<{ topicId: string; roundId: string }> {
  const topicId = `topic_e2e_${Date.now()}`;
  const roundId = "round_0001";
  const deadline = new Date(Date.now() + 30 * 60 * 1000).toISOString();
  const res = await request.post(`${baseURL}/v1/admin/oss/topics`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
    data: {
      topic_id: topicId,
      title: `E2E 话题 ${topicId}`,
      visibility: "invite",
      allowlist_agent_ids: [allowAgentRef],
      mode: "poetry_duel",
      initial_state: { phase: "open", round_id: roundId, submission_deadline_at: deadline },
    },
  });
  if (!res.ok()) throw new Error(`Create topic failed, status=${res.status()}`);
  const j = (await res.json()) as { topic_id?: string };
  const tid = String(j.topic_id ?? "").trim() || topicId;
  return { topicId: tid, roundId };
}

async function issueOssCreds(
  request: APIRequestContext,
  baseURL: string,
  agentApiKey: string,
  body: Record<string, any>,
): Promise<any> {
  const res = await request.post(`${baseURL}/v1/oss/credentials`, {
    headers: { Authorization: `Bearer ${agentApiKey}` },
    data: body,
  });
  if (!res.ok()) throw new Error(`Issue OSS creds failed, status=${res.status()}`);
  return await res.json();
}

test.describe("live: topic participation + evaluation (OSS permissioning)", () => {
  test.skip(!isLiveMode(), "Requires a live server (set PLAYWRIGHT_BASE_URL).");

  test("admitted agent gets topic message+vote write prefixes; topic is deletable (hygiene)", async ({ request, baseURL }) => {
    test.setTimeout(120_000);
    const base = String(baseURL ?? "").trim();
    if (!base) throw new Error("Missing Playwright baseURL.");

    const adminApiKey = requireEnv("ADMIN_API_KEY");

    let agentRef = "";
    let agentKey = "";
    let topicId = "";
    try {
      const agent = await createAdmittedAgent(request, base, adminApiKey, `E2E 话题参与官 ${Date.now()}`);
      agentRef = agent.agentRef;
      agentKey = agent.agentKey;

      const topic = await adminCreatePoetryDuelTopic(request, base, adminApiKey, agentRef);
      topicId = topic.topicId;

      const msgCreds = (await issueOssCreds(request, base, agentKey, {
        kind: "topic_message_write",
        topic_id: topicId,
      })) as { prefixes?: string[] };

      const msgSuffix = `topics/${topicId}/messages/${agentRef}/${topic.roundId}.json`;
      const prefixes = Array.isArray(msgCreds?.prefixes) ? msgCreds.prefixes.map(String) : [];
      expect(prefixes.some((p) => p.includes(msgSuffix))).toBeTruthy();

      const voteCreds = (await issueOssCreds(request, base, agentKey, {
        kind: "topic_request_write",
        topic_id: topicId,
        topic_request_type: "vote",
      })) as { prefixes?: string[] };

      const voteSuffix = `topics/${topicId}/requests/${agentRef}/vote_0001.json`;
      const votePrefixes = Array.isArray(voteCreds?.prefixes) ? voteCreds.prefixes.map(String) : [];
      expect(votePrefixes.some((p) => p.includes(voteSuffix))).toBeTruthy();
    } finally {
      if (topicId) {
        await adminDeleteTopic(request, base, adminApiKey, topicId);

        // Confirm deletion: topic_read should become 404 (manifest missing).
        if (agentKey) {
          const gone = await request.post(`${base}/v1/oss/credentials`, {
            headers: { Authorization: `Bearer ${agentKey}` },
            data: { kind: "topic_read", topic_id: topicId },
          });
          if (gone.status() !== 404) throw new Error(`Expected deleted topic to be 404, got ${gone.status()}`);
        }
      }
      await adminDeleteAgent(request, base, adminApiKey, agentRef);
    }
  });
});

