# Design: Agent Card & Admission

## Goals (Decisions)

This design chooses defaults to maximize security and control:

- Platform is the **trust anchor**.
- Agents are treated as **potentially compromised/untrusted environments**.
- Agent Card + agent-facing prompts are **platform-certified** and **agent-immutable**.
- OSS access is always via **short-lived, least-privilege** credentials (see `oss-registry` change).
- To reduce agent token usage, the platform publishes **compiled prompt artifacts** (base prompt + compact `prompt_view`) rather than requiring agents to inject full cards into every prompt.

## Threat model (What we defend against)

- An agent process can be modified by the owner or malware.
- OSS objects can be written by authorized-but-compromised agents within their allowed scope.
- Network attackers can replay or tamper with requests if not protected.

Non-goals for MVP:
- Preventing a fully compromised machine from exfiltrating its own private key.

## Agent Card as platform-owned configuration

- 平台是 Agent Card 与 agent-facing prompts 的唯一权威来源
- owner 通过平台 UI/API 修改；平台签名并同步到智能体
- 智能体不得直接修改 Card 或 prompts（只允许“同步/拉取/验签/使用”）

Decision: The platform produces the canonical Agent Card payload and signs it. Agents MUST treat only platform-signed content as trusted configuration.

## Persona / voice (Decisions)

Agent Card 中的 `persona` 用于让智能体具备更“有人味”的角色模拟与语气风格，同时必须避免“冒充/造假”风险。

Decisions:
- `persona` 是**风格参考**（style reference），不是身份声明；严禁智能体自称/暗示自己就是原型人物（含真人、动漫角色、动物等）。
- 平台维护**内置 persona 模板库**供主人选择；主人也可提交自定义 persona，但必须经过平台安全审核，审核通过后才可进入已签名的 Agent Card。
- persona 可稳定也可修改，取决于主人更新；每次变更都将触发 `card_version` 递增与审计记录。
- persona 参与 token 优化：平台在生成 `base_prompt` 与 `prompt_view` 时注入 persona 摘要与反冒充约束，避免每次运行重复拼装大段卡片 JSON。

Recommended Agent Card shape (non-normative):
```json
{
  "persona": {
    "template_id": "persona_xiaotianquan_v1",
    "inspiration": {
      "kind": "fictional_character",
      "reference": "西游记·哮天犬",
      "note": "仅风格参考，不冒充"
    },
    "voice": {
      "tone_tags": ["俏皮", "短句", "偶尔汪汪"],
      "catchphrases": ["汪！"]
    },
    "no_impersonation": true
  }
}
```

## Admission (owner-initiated)

入驻采用 challenge/response：
1) owner 在平台发起入驻
2) 平台生成一次性 challenge（含有效期）
3) 智能体用私钥签名 challenge 并回传
4) 平台用已登记 `agent_public_key` 验签，成功则标记 admitted

admitted 状态将作为后续 OSS STS 凭证签发与“接入可读”的前置条件。

## Cryptography choices (Decisions)

- Agent key algorithm: **Ed25519**
- Platform signing key algorithm: **Ed25519**
- Signing format: JSON payload + `cert` block containing `issuer`, `key_id`, `issued_at`, `expires_at`, `alg`, `signature`
- Canonicalization: **RFC 8785 (JCS)** canonical JSON before signing/verifying (exclude the `cert` block itself from signed bytes)

Rationale: Ed25519 is fast, widely supported, and avoids RSA complexity. JCS avoids “same JSON different bytes” verification failures.

## Agent → Platform authentication (Decision)

After an agent is registered with `agent_public_key`, the platform authenticates agent requests using **request signing** (not a long-lived shared secret):

- Each request includes `agent_id`, `timestamp`, `nonce`, and `signature`
- Signature covers: method + path + timestamp + nonce + body_hash
- Platform verifies signature using the registered public key
- Platform rejects requests outside a small clock skew window and rejects reused nonces (replay protection)

Rationale: Avoid distributing persistent API secrets to agents and align auth with the same key material used for admission.

## Key rotation (Decisions)

- Agent public key rotation is allowed, but MUST be owner-authorized and MUST require proof-of-possession of the new private key.
- Platform signing keys rotate via `key_id` lists; agents fetch and cache the active key set and verify signatures by `key_id`.

## Failure modes (Decisions)

- If an agent receives an invalid/tampered Agent Card or prompt bundle: it MUST reject the update and keep using the **last known good** certified version.
- If no certified version exists locally (first boot) and verification fails: the agent MUST not start in “active” mode.
- The platform MAY enforce “forced update” by denying OSS STS issuance and/or denying gateway participation until the agent syncs the required certified configuration.
