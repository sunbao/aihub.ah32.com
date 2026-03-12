import { expect, test } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { isLiveMode, requireEnv } from "./helpers/liveAuth";
import { keepE2EData, recordKeptData } from "./helpers/keepData";

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

async function createAgent(
  request: APIRequestContext,
  baseURL: string,
  adminApiKey: string,
  name: string,
): Promise<{ agentRef: string; agentKey: string }> {
  const create = await request.post(`${baseURL}/v1/agents`, {
    headers: { Authorization: `Bearer ${adminApiKey}` },
    data: {
      name,
      description: "e2e: topic participation (gateway write)",
      tags: ["e2e", "topic-flow"],
    },
  });
  if (!create.ok()) throw new Error(`Create agent failed, status=${create.status()}`);
  const cj = (await create.json()) as { agent_ref?: string; api_key?: string };
  const agentRef = String(cj.agent_ref ?? "").trim();
  const agentKey = String(cj.api_key ?? "").trim();
  if (!agentRef || !agentKey) throw new Error("Create agent response missing agent_ref/api_key.");
  return { agentRef, agentKey };
}

async function adminCreatePoetryDuelTopic(
  request: APIRequestContext,
  baseURL: string,
  adminApiKey: string,
  allowAgentRef: string,
): Promise<{ topicId: string; roundId: string }> {
  // Topic creation requires platform signing keys. Ensure they exist.
  {
    const keysRes = await request.get(`${baseURL}/v1/admin/platform/signing-keys`, {
      headers: { Authorization: `Bearer ${adminApiKey}` },
    });
    if (!keysRes.ok()) {
      const body = await keysRes.text();
      throw new Error(`List platform signing keys failed, status=${keysRes.status()} body=${body.slice(0, 600)}`);
    }
    const keysJson = (await keysRes.json()) as { keys?: any[] };
    const keys = Array.isArray(keysJson?.keys) ? keysJson.keys : [];
    if (keys.length === 0) {
      const rot = await request.post(`${baseURL}/v1/admin/platform/signing-keys/rotate`, {
        headers: { Authorization: `Bearer ${adminApiKey}` },
      });
      if (!rot.ok()) {
        const body = await rot.text();
        throw new Error(`Rotate platform signing key failed, status=${rot.status()} body=${body.slice(0, 600)}`);
      }
    }
  }

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
      rules: { judge_mode: "vote" },
      initial_state: { phase: "open", round_id: roundId, submission_deadline_at: deadline },
    },
  });
  if (!res.ok()) {
    const body = await res.text();
    throw new Error(`Create topic failed, status=${res.status()} body=${body.slice(0, 600)}`);
  }
  const j = (await res.json()) as { topic_id?: string };
  const tid = String(j.topic_id ?? "").trim() || topicId;
  return { topicId: tid, roundId };
}

test.describe("live: topic participation (gateway write)", () => {
  test.skip(!isLiveMode(), "Requires a live server (set PLAYWRIGHT_BASE_URL).");

  test("agent can write topic message + request via gateway; topic is deletable (hygiene)", async ({ request, baseURL }) => {
    test.setTimeout(120_000);
    const base = String(baseURL ?? "").trim();
    if (!base) throw new Error("Missing Playwright baseURL.");

    const adminApiKey = requireEnv("ADMIN_API_KEY");

    let agentRef = "";
    let agentKey = "";
    let topicId = "";
    const agentName = `E2E 话题参与测试 ${Date.now()}`;
    try {
      const agent = await createAgent(request, base, adminApiKey, agentName);
      agentRef = agent.agentRef;
      agentKey = agent.agentKey;

      const topic = await adminCreatePoetryDuelTopic(request, base, adminApiKey, agentRef);
      topicId = topic.topicId;

      const msgText = `E2E message ${Date.now()} ${Math.random().toString(16).slice(2)}`;
      {
        const res = await request.post(`${base}/v1/gateway/topics/${encodeURIComponent(topicId)}/messages`, {
          headers: { Authorization: `Bearer ${agentKey}` },
          data: { content: { text: msgText } },
        });
        if (!res.ok()) {
          const body = await res.text();
          throw new Error(`Write topic message failed, status=${res.status()} body=${body.slice(0, 600)}`);
        }
      }

      {
        const res = await request.post(`${base}/v1/gateway/topics/${encodeURIComponent(topicId)}/requests`, {
          headers: { Authorization: `Bearer ${agentKey}` },
          data: { type: "vote", payload: { round_id: topic.roundId, choice: "up" } },
        });
        if (!res.ok()) {
          const body = await res.text();
          throw new Error(`Write topic request failed, status=${res.status()} body=${body.slice(0, 600)}`);
        }
      }

      // Verify message is visible in the topic thread (as an authenticated owner for invite topics).
      {
        const res = await request.get(`${base}/v1/topics/${encodeURIComponent(topicId)}/thread?limit=50`, {
          headers: { Authorization: `Bearer ${adminApiKey}` },
        });
        if (!res.ok()) {
          const body = await res.text();
          throw new Error(`Get topic thread failed, status=${res.status()} body=${body.slice(0, 600)}`);
        }
        const j = (await res.json()) as { messages?: { text?: string }[] };
        const msgs = Array.isArray(j?.messages) ? j.messages : [];
        expect(msgs.some((m) => String(m?.text ?? "").includes(msgText))).toBeTruthy();
      }
    } finally {
      if (keepE2EData()) {
        recordKeptData({ kind: "topic", suite: "live-topic-flow", topic_id: topicId, agent_ref: agentRef, agent_name: agentName });
      } else {
        if (topicId) {
          await adminDeleteTopic(request, base, adminApiKey, topicId);
        }
        await adminDeleteAgent(request, base, adminApiKey, agentRef);
      }
    }
  });
});

