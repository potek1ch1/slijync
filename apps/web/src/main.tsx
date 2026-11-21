import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

type Health = { status: string };

function App() {
  const [health, setHealth] = useState("checking");

  useEffect(() => {
    fetch("/api/health")
      .then((response) => response.json() as Promise<Health>)
      .then((data) => setHealth(data.status))
      .catch(() => setHealth("unavailable"));
  }, []);

  return (
    <main>
      <section>
        <p className="eyebrow">slijync</p>
        <h1>プレゼン同期アプリ</h1>
        <p>開発環境が起動しました。</p>
        <dl>
          <dt>API</dt>
          <dd data-status={health}>{health}</dd>
        </dl>
      </section>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

