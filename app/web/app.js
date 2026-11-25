// myTraffic UI — talks to the local API on the same origin.
(function () {
  "use strict";
  const $ = (s) => document.querySelector(s);
  const el = (t, props, kids) => {
    const n = document.createElement(t);
    if (props) Object.assign(n, props);
    (kids || []).forEach((k) => n.append(k));
    return n;
  };
  const esc = (s) => (s == null ? "" : String(s).replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c])));

  // ---- theme ----
  $("#themeBtn").addEventListener("click", () => {
    const cur = document.documentElement.getAttribute("data-theme") ||
      (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
    const next = cur === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    try { localStorage.setItem("mt-theme", next); } catch (e) {}
  });

  // ---- tabs ----
  document.querySelectorAll(".tab").forEach((tab) => {
    tab.addEventListener("click", () => {
      document.querySelectorAll(".tab").forEach((t) => t.classList.remove("active"));
      document.querySelectorAll(".view").forEach((v) => v.classList.remove("active"));
      tab.classList.add("active");
      $("#view-" + tab.dataset.view).classList.add("active");
      if (tab.dataset.view === "intercept") refreshQueue();
      if (tab.dataset.view === "rules") loadRules();
      if (tab.dataset.view === "scope") { loadScope(); loadUpstream(); }
      if (tab.dataset.view === "sitemap") loadSitemap();
      if (tab.dataset.view === "websockets") loadWS();
      if (tab.dataset.view === "findings") loadFindings();
    });
  });

  const api = {
    get: (u) => fetch(u).then((r) => r.json()),
    post: (u, b) => fetch(u, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(b) }).then((r) => r.json()),
    put: (u, b) => fetch(u, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(b) }).then((r) => r.json()),
    del: (u) => fetch(u, { method: "DELETE" }).then((r) => r.json()),
  };

  // ---- history ----
  let flows = [];        // live, unfiltered (newest first)
  let filtered = null;   // server-filtered snapshot, or null when no filter
  let selected = null;
  const rows = $("#flowRows");
  const filterInput = $("#filterInput");

  function statusClass(s) { return s >= 500 ? "s5" : s >= 400 ? "s4" : s >= 300 ? "s3" : "s2"; }

  function render() {
    const list = filtered !== null ? filtered : flows;
    rows.textContent = "";
    for (const f of list) {
      const tr = el("tr");
      if (selected === f.id) tr.classList.add("sel");
      else if (f.color) { tr.style.background = f.color; tr.style.color = "#0C0F14"; }
      const star = f.starred ? "★ " : "";
      tr.innerHTML =
        `<td>${star}${f.id}</td><td>${esc(f.method)}</td><td>${esc(f.host)}</td>` +
        `<td>${esc(f.path)}</td><td class="${statusClass(f.status)}">${f.status}</td>` +
        `<td>${f.size}</td><td>${f.durationMs}ms</td>`;
      tr.addEventListener("click", () => { selected = f.id; render(); showDetail(f); });
      rows.append(tr);
    }
    $("#count").textContent = filtered !== null ? (list.length + " matched") : (flows.length + " total");
  }

  // Debounced server-side HTTPQL filtering.
  let filterTimer = null;
  async function applyFilter() {
    const q = filterInput.value.trim();
    if (!q) { filtered = null; render(); return; }
    try {
      filtered = (await api.get("/api/flows?limit=2000&filter=" + encodeURIComponent(q))) || [];
      filterInput.style.borderColor = "";
    } catch (e) {
      filterInput.style.borderColor = "var(--err)";
    }
    render();
  }

  function showDetail(f) {
    const d = $("#detail");
    const reqH = Object.entries(f.reqHeaders || {}).map(([k, v]) => `${k}: ${v}`).join("\n");
    const resH = Object.entries(f.resHeaders || {}).map(([k, v]) => `${k}: ${v}`).join("\n");
    d.innerHTML =
      `<div class="detail-actions">
         <button class="btn" id="toReplay">Send to Replay</button>
         <button class="btn-ghost" id="toAutomate">Send to Automate</button>
         <button class="btn-ghost" id="quickReplay">Replay now</button>
         <button class="btn-ghost" id="flowToFinding">+ Finding</button>
         <button class="btn-ghost" id="starFlow">${f.starred ? "★" : "☆"}</button>
       </div>
       <div class="detail-actions" style="align-items:center">
         <span class="muted" style="font-size:12px">Color:</span>
         ${["", "#FDEBB0", "#EEE5E0", "#E9E4E0", "#B08A8A", "#A87C3A"].map((c) => `<button data-color="${c}" title="${c || "none"}" style="width:22px;height:22px;border-radius:6px;border:1px solid var(--line);cursor:pointer;background:${c || "transparent"}">${c ? "" : "×"}</button>`).join("")}
       </div>
       <textarea id="flowNote" placeholder="note…" spellcheck="false" style="width:100%;min-height:48px;margin:8px 0">${esc(f.note || "")}</textarea>
       <button class="btn-ghost" id="saveNote">Save note</button>
       <h4>Request</h4><pre>${esc(f.method)} ${esc(f.path)}\n${esc(reqH)}\n\n${esc(f.reqBody || "")}</pre>
       <h4>Response — ${f.status}</h4><pre>${esc(resH)}\n\n${esc(f.resBody || "")}</pre>`;
    $("#toReplay").addEventListener("click", () => {
      $("#rMethod").value = f.method;
      $("#rURL").value = f.scheme + "://" + f.host + f.path;
      $("#rHeaders").value = reqH;
      $("#rBody").value = f.reqBody || "";
      document.querySelector('.tab[data-view="replay"]').click();
    });
    $("#quickReplay").addEventListener("click", async () => {
      const r = await api.post("/api/replay", { id: f.id });
      alert("Replayed #" + f.id + " → " + (r.status || r.error));
    });
  }

  filterInput.addEventListener("input", () => {
    clearTimeout(filterTimer);
    filterTimer = setTimeout(applyFilter, 250);
  });

  $("#clearBtn").addEventListener("click", async () => {
    await api.del("/api/flows");
    flows = []; filtered = null; selected = null; render();
  });

  async function loadInitial() {
    try { flows = (await api.get("/api/flows?limit=1000")) || []; render(); } catch (e) {}
  }

  // ---- live stream ----
  function connect() {
    const es = new EventSource("/api/stream");
    es.onopen = () => setConn(true);
    es.onerror = () => { setConn(false); };
    es.onmessage = (ev) => {
      try {
        const f = JSON.parse(ev.data);
        flows.unshift(f);
        if (flows.length > 2000) flows.pop();
        if (filtered === null) render(); // don't disturb an active filter snapshot
      } catch (e) {}
    };
  }
  function setConn(on) {
    const p = $("#connPill");
    p.textContent = on ? "live" : "offline";
    p.className = "pill " + (on ? "on" : "off");
  }

  // ---- intercept ----
  const interceptToggle = $("#interceptToggle");
  interceptToggle.addEventListener("change", async () => {
    await api.post("/api/intercept", { enabled: interceptToggle.checked });
    refreshQueue();
  });
  async function syncIntercept() {
    try { const r = await api.get("/api/intercept"); interceptToggle.checked = !!r.enabled; } catch (e) {}
  }
  async function refreshQueue() {
    let q = [];
    try { q = (await api.get("/api/intercept/queue")) || []; } catch (e) {}
    const badge = $("#qBadge");
    if (q.length) { badge.hidden = false; badge.textContent = q.length; } else { badge.hidden = true; }
    const box = $("#queue");
    box.textContent = "";
    if (!q.length) { box.append(el("p", { className: "muted", textContent: "No paused requests." })); return; }
    for (const it of q) {
      const wrap = el("div", { className: "qitem" });
      const hdrs = Object.entries(it.header || {}).map(([k, v]) => `${k}: ${v}`).join("\n");
      wrap.innerHTML =
        `<div class="qhead">${esc(it.method)} ${esc(it.url)}</div>
         <textarea data-h>${esc(hdrs)}</textarea>
         <textarea data-b>${esc(it.body || "")}</textarea>
         <div class="row" style="margin-top:8px">
           <button class="btn" data-fwd>Forward</button>
           <button class="btn-ghost" data-drop>Drop</button>
         </div>`;
      wrap.querySelector("[data-fwd]").addEventListener("click", async () => {
        const header = {};
        wrap.querySelector("[data-h]").value.split("\n").forEach((l) => {
          const i = l.indexOf(":"); if (i > 0) header[l.slice(0, i).trim()] = l.slice(i + 1).trim();
        });
        await api.post("/api/intercept/resolve", { id: it.id, action: "forward", header, body: wrap.querySelector("[data-b]").value });
        refreshQueue();
      });
      wrap.querySelector("[data-drop]").addEventListener("click", async () => {
        await api.post("/api/intercept/resolve", { id: it.id, action: "drop" });
        refreshQueue();
      });
      box.append(wrap);
    }
  }
  setInterval(() => { if ($("#view-intercept").classList.contains("active")) refreshQueue(); }, 1500);

  // ---- replay ----
  $("#rSend").addEventListener("click", async () => {
    const header = {};
    $("#rHeaders").value.split("\n").forEach((l) => {
      const i = l.indexOf(":"); if (i > 0) header[l.slice(0, i).trim()] = l.slice(i + 1).trim();
    });
    $("#rResponse").textContent = "sending…";
    try {
      const r = await api.post("/api/replay", {
        method: $("#rMethod").value, url: $("#rURL").value, header, body: $("#rBody").value,
      });
      if (r.error) { $("#rResponse").textContent = "error: " + r.error; return; }
      const h = Object.entries(r.header || {}).map(([k, v]) => `${k}: ${v}`).join("\n");
      $("#rResponse").textContent = `HTTP ${r.status}  (${r.durationMs}ms)\n${h}\n\n${r.body || ""}`;
    } catch (e) { $("#rResponse").textContent = "error: " + e; }
  });

  // ---- rules ----
  async function loadRules() {
    const list = (await api.get("/api/rules")) || [];
    const tb = $("#ruleRows"); tb.textContent = "";
    for (const r of list) {
      const tr = el("tr");
      tr.innerHTML =
        `<td>${r.enabled ? "✓" : "—"}</td><td>${esc(r.name)}</td><td>${esc(r.target)}</td>` +
        `<td><code>${esc(r.match)}</code></td><td><code>${esc(r.replace)}</code></td>` +
        `<td><button class="link" data-id="${r.id}">delete</button></td>`;
      tr.querySelector("button").addEventListener("click", async () => { await api.del("/api/rules/" + r.id); loadRules(); });
      tb.append(tr);
    }
  }
  $("#ruleAdd").addEventListener("click", async () => {
    const rule = {
      enabled: true, name: $("#ruleName").value || "rule",
      target: $("#ruleTarget").value, match: $("#ruleMatch").value, replace: $("#ruleReplace").value,
    };
    const res = await api.post("/api/rules", rule);
    if (res.error) { alert("Bad regex: " + res.error); return; }
    $("#ruleName").value = $("#ruleMatch").value = $("#ruleReplace").value = "";
    loadRules();
  });

  // ---- decoder ----
  $("#dRun").addEventListener("click", async () => {
    const r = await api.post("/api/decode", { op: $("#dOp").value, input: $("#dInput").value });
    $("#dOutput").textContent = r.error ? "error: " + r.error : r.output;
  });

  // ---- scope ----
  async function loadScope() {
    const r = await api.get("/api/scope");
    $("#scopeInput").value = (r.hosts || []).join("\n");
  }
  $("#scopeSave").addEventListener("click", async () => {
    const hosts = $("#scopeInput").value.split("\n").map((s) => s.trim()).filter(Boolean);
    await api.put("/api/scope", { hosts });
    alert("Scope saved (" + hosts.length + " host" + (hosts.length === 1 ? "" : "s") + ").");
  });

  // ---- automate ----
  $("#aMark").addEventListener("click", () => {
    const ta = $("#aURL");
    const s = ta.selectionStart, e = ta.selectionEnd, v = ta.value;
    ta.value = v.slice(0, s) + "§" + v.slice(s, e) + "§" + v.slice(e);
    ta.focus();
  });
  function parsePayloadBlocks(text) {
    // Split on a line containing only --- ; each block is a set.
    const blocks = text.split(/\n---\n/);
    return blocks.map((blk) => {
      const lines = blk.split("\n").map((l) => l.replace(/\r$/, "")).filter((l) => l !== "");
      if (lines.length === 1 && lines[0].startsWith("num:")) {
        const [from, to, step] = lines[0].slice(4).split(",").map(Number);
        return { type: "numbers", from: from || 0, to: to || 0, step: step || 1 };
      }
      return { type: "list", list: lines };
    });
  }
  $("#aRun").addEventListener("click", async () => {
    const headers = {};
    $("#aHeaders").value.split("\n").forEach((l) => {
      const i = l.indexOf(":"); if (i > 0) headers[l.slice(0, i).trim()] = l.slice(i + 1).trim();
    });
    const spec = {
      method: $("#aMethod").value,
      url: $("#aURL").value,
      headers,
      body: $("#aBody").value,
      attackType: $("#aAttack").value,
      concurrency: +$("#aConc").value || 10,
      payloads: parsePayloadBlocks($("#aPayloads").value),
    };
    const g = $("#aGrep").value.trim().replace(/^"|"$/g, "");
    if (g) spec.grep = [g];
    $("#aStatus").textContent = "running…";
    $("#aRows").textContent = "";
    try {
      const r = await api.post("/api/automate", spec);
      if (r.error) { $("#aStatus").textContent = "error: " + r.error; return; }
      $("#aStatus").textContent = `${r.count} requests, ${r.positions} position(s), ${r.attackType}`;
      const tb = $("#aRows");
      for (const row of r.results) {
        const tr = el("tr");
        let grep = "";
        if (row.grep) for (const k in row.grep) if (row.grep[k]) grep = "✓ " + k;
        tr.innerHTML = `<td>${row.index}</td><td>${esc((row.payloads || []).join(", "))}</td>` +
          `<td class="${statusClass(row.status)}">${row.error ? "ERR" : row.status}</td>` +
          `<td>${row.length}</td><td>${row.durationMs}ms</td><td>${esc(grep)}</td>`;
        tb.append(tr);
      }
    } catch (e) { $("#aStatus").textContent = "error: " + e; }
  });

  // send-to-automate from a selected flow
  document.addEventListener("click", (ev) => {
    if (ev.target && ev.target.id === "toAutomate" && selected) {
      const f = flows.find((x) => x.id === selected);
      if (!f) return;
      $("#aURL").value = f.scheme + "://" + f.host + f.path;
      $("#aMethod").value = f.method;
      $("#aBody").value = f.reqBody || "";
      document.querySelector('.tab[data-view="automate"]').click();
    }
  });

  // ---- comparer ----
  $("#cmpRun").addEventListener("click", async () => {
    const r = await api.post("/api/compare", { a: $("#cmpA").value, b: $("#cmpB").value });
    const out = $("#cmpOut");
    out.textContent = "";
    for (const d of r.diff || []) {
      const line = el("div");
      line.textContent = (d.op === " " ? "  " : d.op + " ") + d.text;
      if (d.op === "-") line.style.color = "var(--err)";
      if (d.op === "+") line.style.color = "var(--ok)";
      if (d.op === "-") line.style.background = "rgba(176,86,78,.08)";
      if (d.op === "+") line.style.background = "rgba(47,125,79,.08)";
      out.append(line);
    }
    if (!(r.diff || []).length) out.textContent = "identical";
  });

  // ---- sitemap ----
  async function loadSitemap() {
    const tree = await api.get("/api/sitemap");
    const box = $("#smTree");
    box.textContent = "";
    function renderNode(name, node, depth) {
      const row = el("div");
      row.style.paddingLeft = depth * 16 + "px";
      const methods = node.methods ? " " + Object.keys(node.methods).join(",") : "";
      row.innerHTML = `<span style="color:var(--gold)">${esc(name)}</span> ` +
        `<span class="muted">(${node.count})${esc(methods)}</span>`;
      box.append(row);
      if (node.children) {
        const keys = Object.keys(node.children).sort();
        for (const k of keys) renderNode(k, node.children[k], depth + 1);
      }
    }
    const hosts = Object.keys(tree).sort();
    if (!hosts.length) { box.textContent = "No traffic captured yet."; return; }
    for (const h of hosts) renderNode(h, tree[h], 0);
  }
  $("#smRefresh").addEventListener("click", loadSitemap);

  // ---- annotations in history detail ----
  async function annotate(id, patch) {
    const f = flows.find((x) => x.id === id) || {};
    const body = Object.assign({ id, color: f.color || "", note: f.note || "", starred: !!f.starred }, patch);
    await api.post("/api/annotate", body);
    Object.assign(f, { color: body.color, note: body.note, starred: body.starred });
    render();
  }
  document.addEventListener("click", (ev) => {
    const t = ev.target;
    if (!t || !selected) return;
    if (t.dataset && t.dataset.color !== undefined) annotate(selected, { color: t.dataset.color });
    if (t.id === "starFlow") { const f = flows.find((x) => x.id === selected); annotate(selected, { starred: !(f && f.starred) }); }
    if (t.id === "saveNote") annotate(selected, { note: $("#flowNote").value });
    if (t.id === "flowToFinding") {
      const f = flows.find((x) => x.id === selected) || {};
      $("#fTitle").value = f.method + " " + f.path;
      $("#fHost").value = f.host || "";
      document.querySelector('.tab[data-view="findings"]').click();
    }
  });

  // ---- sequencer ----
  $("#seqRun").addEventListener("click", async () => {
    const tokens = $("#seqInput").value.split("\n").map((s) => s.trim()).filter(Boolean);
    const r = await api.post("/api/sequencer", { tokens });
    const bar = (v, max) => { const w = Math.min(100, (v / max) * 100); return `<span style="display:inline-block;height:8px;width:${w}%;background:var(--gold);border-radius:4px"></span>`; };
    const maxH = Math.max(1, ...(r.perPosition || [1]));
    $("#seqOut").innerHTML =
      `<div class="card" style="max-width:520px">
        <div><strong>${esc(r.verdict || "")}</strong></div>
        <table style="width:100%;margin-top:10px;font-size:14px">
          <tr><td class="muted">Tokens</td><td>${r.count} (${r.unique} unique)</td></tr>
          <tr><td class="muted">Length</td><td>${r.minLen}–${r.maxLen}</td></tr>
          <tr><td class="muted">Character set</td><td>${r.charsetSize} symbols</td></tr>
          <tr><td class="muted">Bits / char</td><td>${(r.bitsPerChar || 0).toFixed(2)}</td></tr>
          <tr><td class="muted">Effective bits / token</td><td><strong>${(r.bitsPerToken || 0).toFixed(1)}</strong></td></tr>
        </table>
        <div class="lbl">Per-position entropy</div>
        <div style="display:flex;gap:2px;align-items:flex-end;height:60px">
          ${(r.perPosition || []).map((h) => `<div title="${h.toFixed(2)} bits" style="flex:1;background:var(--tint);display:flex;align-items:flex-end"><div style="width:100%;height:${(h / maxH) * 100}%;background:var(--gold)"></div></div>`).join("")}
        </div>
      </div>`;
  });

  // ---- websockets ----
  let wsConnected = false;
  let wsMsgs = [];
  function renderWS() {
    const tb = $("#wsRows"); tb.textContent = "";
    for (const m of wsMsgs) {
      const tr = el("tr");
      const dirColor = m.direction === "client" ? "var(--gold)" : "var(--rose)";
      tr.innerHTML = `<td>${m.id}</td><td style="color:${dirColor}">${m.direction === "client" ? "▲ out" : "▼ in"}</td>` +
        `<td>${esc(m.format)}</td><td>${esc(m.host)}</td><td>${m.len}</td><td>${esc((m.payload || "").slice(0, 120))}</td>`;
      tb.append(tr);
    }
  }
  async function loadWS() {
    const q = $("#wsFilter").value.trim();
    wsMsgs = (await api.get("/api/ws?limit=1000&filter=" + encodeURIComponent(q))) || [];
    renderWS();
    if (!wsConnected) {
      wsConnected = true;
      const es = new EventSource("/api/ws/stream");
      es.onmessage = (ev) => {
        try {
          const m = JSON.parse(ev.data);
          if (!$("#wsFilter").value.trim()) { wsMsgs.unshift(m); if (wsMsgs.length > 1000) wsMsgs.pop(); if ($("#view-websockets").classList.contains("active")) renderWS(); }
        } catch (e) {}
      };
    }
  }
  let wsTimer = null;
  $("#wsFilter").addEventListener("input", () => { clearTimeout(wsTimer); wsTimer = setTimeout(loadWS, 250); });
  $("#wsClear").addEventListener("click", async () => { await api.del("/api/ws"); wsMsgs = []; renderWS(); });

  // ---- findings ----
  async function loadFindings() {
    const list = (await api.get("/api/findings")) || [];
    const tb = $("#fRows"); tb.textContent = "";
    const sevColor = { info: "var(--muted)", low: "var(--taupe)", medium: "var(--gold)", high: "var(--rose)", critical: "var(--err)" };
    for (const f of list) {
      const tr = el("tr");
      tr.innerHTML = `<td style="color:${sevColor[f.severity] || "var(--muted)"}">${esc(f.severity)}</td>` +
        `<td>${esc(f.title)}</td><td>${esc(f.host || "")}</td><td>${f.flowId || ""}</td><td>${esc(f.note || "")}</td>` +
        `<td><button class="link" data-id="${f.id}">delete</button></td>`;
      tr.querySelector("button").addEventListener("click", async () => { await api.del("/api/findings/" + f.id); loadFindings(); });
      tb.append(tr);
    }
  }
  $("#fAdd").addEventListener("click", async () => {
    const f = { title: $("#fTitle").value, severity: $("#fSeverity").value, host: $("#fHost").value, note: $("#fNote").value };
    if (!f.title) { alert("Title required"); return; }
    await api.post("/api/findings", f);
    $("#fTitle").value = $("#fHost").value = $("#fNote").value = "";
    loadFindings();
  });

  // ---- project save / load ----
  $("#saveProject").addEventListener("click", () => {
    const a = document.createElement("a");
    a.href = "/api/project/export";
    a.download = "mytraffic-project.json";
    a.click();
  });
  $("#loadProject").addEventListener("click", () => $("#loadFile").click());
  $("#loadFile").addEventListener("change", async (ev) => {
    const file = ev.target.files[0];
    if (!file) return;
    const text = await file.text();
    try {
      const res = await fetch("/api/project/import", { method: "POST", headers: { "Content-Type": "application/json" }, body: text });
      const r = await res.json();
      flows = (await api.get("/api/flows?limit=2000")) || [];
      filtered = null; render();
      alert("Loaded " + (r.flows || 0) + " flows, " + (r.findings || 0) + " findings.");
    } catch (e) { alert("Load failed: " + e); }
    ev.target.value = "";
  });

  // ---- upstream proxy ----
  async function loadUpstream() {
    try { const r = await api.get("/api/upstream"); $("#upstreamInput").value = r.upstream || ""; } catch (e) {}
  }
  $("#upstreamSave").addEventListener("click", async () => {
    await api.put("/api/upstream", { upstream: $("#upstreamInput").value.trim() });
    alert("Upstream proxy saved.");
  });

  // ---- boot ----
  loadInitial();
  connect();
  syncIntercept();
})();
