function formatLogTime(timeStr) {
  try {
    const d = new Date(timeStr);
    return `${d.toLocaleTimeString()}.${String(d.getMilliseconds()).padStart(3, "0")}`;
  } catch {
    return "";
  }
}

function logMessage(item) {
  switch (item.event) {
    case "config_reload_started":
      return {
        tone: "pending",
        label: "RELOAD",
        text: `Config reload started, generation ${item.previous_config_generation} -> ${item.config_generation || "?"}`
      };
    case "config_reload_succeeded":
      return { tone: "ok", label: "RELOAD", text: `Config reloaded, generation ${item.config_generation}` };
    case "config_reload_failed":
      return { tone: "error", label: "RELOAD", text: `Config reload failed: ${item.reason || item.error_class}` };
    case "request_started":
      return { tone: "info", label: "INBOUND", text: `${item.client || "client"} -> ${item.route || "route"}` };
    case "route_planned":
      return { tone: "info", label: "PLAN", text: `Routing strategy: ${item.strategy}` };
    case "target_selected":
      return { tone: "selected", label: "TARGET", text: `${item.model} on ${item.provider}` };
    case "upstream_request_started":
      return { tone: "info", label: "UPSTREAM", text: `Dispatching to ${item.upstream_model}` };
    case "upstream_response":
      return { tone: "ok", label: "RESPONSE", text: `Status ${item.status}, latency ${item.latency_ms}ms` };
    case "request_finished":
      return { tone: "muted", label: "DONE", text: `Status ${item.status}, total ${item.latency_ms}ms` };
    case "request_failed":
      return { tone: "error", label: "FAILED", text: `${item.reason || item.error_class}, latency ${item.latency_ms}ms` };
    default:
      return { tone: "muted", label: item.event || "LOG", text: item.msg || JSON.stringify(item) };
  }
}

export default function LogLine({ item }) {
  const log = logMessage(item);
  return (
    <div className={`log-line ${log.tone}`}>
      <time>{formatLogTime(item.time)}</time>
      <span className="log-label">{log.label}</span>
      <p>{log.text}</p>
    </div>
  );
}
