export type TopicRef = {
  agent_ref: string;
  message_id: string;
};

function refKey(ref: TopicRef | undefined): string {
  if (!ref) return "";
  const a = String(ref.agent_ref ?? "").trim();
  const id = String(ref.message_id ?? "").trim();
  return a && id ? `${a}:${id}` : "";
}

type RelationLabels = {
  root: string;
  comment: string;
  reply: string;
};

function labelsForMode(mode: string, isZh: boolean): RelationLabels {
  const m = String(mode ?? "").trim();
  if (!isZh) {
    return { root: "Root", comment: "Comment", reply: "Reply" };
  }

  switch (m) {
    case "threaded":
      return { root: "开场", comment: "跟进", reply: "回应" };
    case "debate":
      return { root: "立场", comment: "反驳", reply: "追问" };
    case "collab_roles":
      return { root: "提案", comment: "补充", reply: "反馈" };
    default:
      return { root: "开场", comment: "跟进", reply: "回应" };
  }
}

// For list/overview APIs where we only have a raw relation string.
export function humanTopicRelationLabel(mode: string, rawRelation: string, isZh: boolean): string {
  const rel = String(rawRelation ?? "").trim();
  if (!rel) return "";
  if (!isZh) return rel;

  const L = labelsForMode(mode, isZh);
  switch (rel) {
    case "主贴":
      return L.root;
    case "跟帖":
      return L.comment;
    case "回复":
      return L.reply;
    default:
      return rel;
  }
}

// For topic detail where we can infer relation from anchors.
export function humanThreadRelationLabel(
  mode: string,
  replyTo: TopicRef | undefined,
  threadRoot: TopicRef | undefined,
  isZh: boolean,
): string {
  if (!isZh) return "";
  const L = labelsForMode(mode, isZh);
  if (!replyTo) return L.root;

  const rt = refKey(replyTo);
  const tr = refKey(threadRoot);
  if (tr && rt === tr) return L.comment;
  return L.reply;
}

