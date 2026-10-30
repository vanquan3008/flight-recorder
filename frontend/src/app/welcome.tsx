import { Activity, ArrowRight, Gauge, KeyRound, RadioTower, Sparkles, Terminal } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "./auth";
import "./welcome.css";

const devKey = import.meta.env.DEV ? "tracepilot-development-bootstrap-key-000000" : "";

const features = [
  { icon: RadioTower, title: "Unified Ingestion", text: "OTLP traces, logs & metrics via :4317/:4318." },
  { icon: Activity, title: "Live Waterfall", text: "Sub-millisecond span request trace flow." },
  { icon: Sparkles, title: "Instant RCA", text: "Ranked hypotheses and incident analysis." },
];

export function Welcome() {
  const { connect } = useAuth();
  const [keyInput, setKeyInput] = useState("");

  return <section className="welcome">
    <div className="welcome-hero">
      <span className="welcome-badge"><Sparkles size={12} aria-hidden="true" /> OpenTelemetry Observability</span>
      <div className="welcome-icon-wrap" aria-hidden="true"><Gauge size={28} /></div>
      <h1>Welcome to TracePilot</h1>
      <p className="welcome-subtitle">Connect the bootstrap/admin key for first-time setup, or a read-scoped key for daily investigation.</p>

      <form className="welcome-key-card" onSubmit={(event) => { event.preventDefault(); connect(keyInput); }}>
        <div className="welcome-input-row">
          <KeyRound size={16} className="welcome-key-icon" aria-hidden="true" />
          <label htmlFor="welcome-key" className="sr-only">API key</label>
          <input id="welcome-key" type="password" autoComplete="off" placeholder="Paste your TracePilot API key…" value={keyInput} onChange={(event) => setKeyInput(event.target.value)} />
          <button type="submit" className="welcome-submit" disabled={!keyInput.trim()}>Connect Console <ArrowRight size={14} aria-hidden="true" /></button>
        </div>
        <div className="welcome-dev-hint">
          <span>Your key stays in this browser session only.</span>
          {devKey && <button type="button" className="welcome-quick-fill" onClick={() => connect(devKey)}><Terminal size={12} aria-hidden="true" /> Auto-fill bootstrap dev key</button>}
        </div>
      </form>

      <ul className="welcome-features">
        {features.map(({ icon: Icon, title, text }) => <li key={title} className="welcome-feat-card">
          <span className="welcome-feat-icon"><Icon size={16} aria-hidden="true" /></span>
          <strong>{title}</strong>
          <small>{text}</small>
        </li>)}
      </ul>

      <Link to="/flow" className="welcome-learn-more">Explore architectural flow &amp; pipeline design <ArrowRight size={13} aria-hidden="true" /></Link>
    </div>
  </section>;
}
