import { CadenceIPC, type CadenceHistoryItem } from "./ipc";

export interface HistoryViewFilter {
  searchQuery: string;
  selectedApp: string;
}

let historyCache: CadenceHistoryItem[] = [];
let activeFilter: HistoryViewFilter = {
  searchQuery: "",
  selectedApp: "all",
};

export async function fetchHistory(): Promise<CadenceHistoryItem[]> {
  try {
    const records = await CadenceIPC.getHistory(50);
    if (records && records.length > 0) {
      historyCache = records;
      return records;
    }
  } catch (err) {
    console.warn("Failed to fetch history via IPC:", err);
  }

  // Provide initial mock/demo history if none exists yet
  if (historyCache.length === 0) {
    historyCache = [
      {
        id: "sess-101",
        text: "func handleAudioChunk(chunk audio.Chunk) error",
        raw_text: "funk handle audio chunk",
        app: "Visual Studio Code",
        duration_ms: 1850,
        created_at: new Date(Date.now() - 3 * 60 * 1000).toISOString(),
      },
      {
        id: "sess-102",
        text: "git commit -m 'feat: implement real-time streaming partials'",
        raw_text: "git commit dash m feat implement real time streaming partials",
        app: "Terminal",
        duration_ms: 2400,
        created_at: new Date(Date.now() - 15 * 60 * 1000).toISOString(),
      },
      {
        id: "sess-103",
        text: "Hey team, the desktop UI scaffold and UNIX socket IPC bridge are now live.",
        raw_text: "hey team the desktop ui scaffold and unix socket ipc bridge are now live",
        app: "Slack",
        duration_ms: 3200,
        created_at: new Date(Date.now() - 45 * 60 * 1000).toISOString(),
      },
      {
        id: "sess-104",
        text: "SELECT * FROM sessions WHERE created_at > datetime('now', '-1 day');",
        raw_text: "select star from sessions where created at greater than datetime now minus one day",
        app: "TablePlus",
        duration_ms: 2900,
        created_at: new Date(Date.now() - 120 * 60 * 1000).toISOString(),
      },
    ];
  }
  return historyCache;
}

export function filterHistoryItems(
  items: CadenceHistoryItem[],
  filter: HistoryViewFilter
): CadenceHistoryItem[] {
  return items.filter((item) => {
    const q = filter.searchQuery.trim().toLowerCase();
    const matchesSearch =
      !q ||
      (item.text && item.text.toLowerCase().includes(q)) ||
      (item.raw_text && item.raw_text.toLowerCase().includes(q)) ||
      (item.app && item.app.toLowerCase().includes(q));

    const matchesApp =
      filter.selectedApp === "all" ||
      (item.app && item.app.toLowerCase() === filter.selectedApp.toLowerCase());

    return matchesSearch && matchesApp;
  });
}

function formatRelativeTime(dateStr?: string): string {
  if (!dateStr) return "recent";
  const date = new Date(dateStr);
  const now = new Date();
  const diffSec = Math.floor((now.getTime() - date.getTime()) / 1000);

  if (diffSec < 60) return "Just now";
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  return date.toLocaleDateString();
}

export function renderHistoryView(
  container: HTMLElement,
  onRepasteAction?: (text: string) => void
): void {
  const filtered = filterHistoryItems(historyCache, activeFilter);

  // Extract unique applications for filter dropdown
  const apps = Array.from(
    new Set(historyCache.map((item) => item.app || "Unknown").filter(Boolean))
  );

  container.innerHTML = `
    <div class="history-panel">
      <div class="history-header">
        <div>
          <h3>Transcription History</h3>
          <p class="history-subtitle">Search, filter, copy, and re-paste past speech-to-text dictations.</p>
        </div>
        <div class="history-toolbar">
          <input
            id="history-search-input"
            type="search"
            class="history-search"
            placeholder="Search transcriptions..."
            value="${activeFilter.searchQuery}"
          />
          <select id="history-app-filter" class="history-filter-select">
            <option value="all" ${activeFilter.selectedApp === "all" ? "selected" : ""}>All Applications</option>
            ${apps
              .map(
                (a) =>
                  `<option value="${a}" ${activeFilter.selectedApp === a ? "selected" : ""}>${a}</option>`
              )
              .join("")}
          </select>
          <button id="btn-refresh-history" class="action-btn" style="background: #475569;">Refresh</button>
        </div>
      </div>

      <div class="history-list" id="history-list">
        ${
          filtered.length === 0
            ? `<div class="history-empty">No matching transcription sessions found.</div>`
            : filtered
                .map((item) => {
                  const durationSec = item.duration_ms
                    ? (item.duration_ms / 1000).toFixed(1) + "s"
                    : "";
                  return `
            <div class="history-card" data-id="${item.id}">
              <div class="history-card-header">
                <div class="history-meta">
                  ${item.app ? `<span class="history-app-tag">${item.app}</span>` : ""}
                  ${durationSec ? `<span class="history-duration">${durationSec}</span>` : ""}
                  <span class="history-time">${formatRelativeTime(item.created_at)}</span>
                </div>
                <div class="history-card-actions">
                  <button class="action-btn copy-btn" data-text="${encodeURIComponent(item.text || "")}">Copy</button>
                  <button class="action-btn repaste-btn" style="background: #10b981;" data-text="${encodeURIComponent(item.text || "")}">Re-paste</button>
                </div>
              </div>
              <div class="history-text">${item.text || item.raw_text || "(empty)"}</div>
              ${
                item.raw_text && item.raw_text !== item.text
                  ? `<div class="history-raw">Raw: ${item.raw_text}</div>`
                  : ""
              }
            </div>
          `;
                })
                .join("")
        }
      </div>
    </div>
  `;

  // Attach search input listener
  const searchInput = container.querySelector("#history-search-input") as HTMLInputElement | null;
  if (searchInput) {
    searchInput.oninput = () => {
      activeFilter.searchQuery = searchInput.value;
      renderHistoryView(container, onRepasteAction);
    };
  }

  // Attach app filter listener
  const appFilter = container.querySelector("#history-app-filter") as HTMLSelectElement | null;
  if (appFilter) {
    appFilter.onchange = () => {
      activeFilter.selectedApp = appFilter.value;
      renderHistoryView(container, onRepasteAction);
    };
  }

  // Attach refresh button listener
  const refreshBtn = container.querySelector("#btn-refresh-history") as HTMLButtonElement | null;
  if (refreshBtn) {
    refreshBtn.onclick = async () => {
      refreshBtn.disabled = true;
      refreshBtn.textContent = "Loading...";
      await fetchHistory();
      renderHistoryView(container, onRepasteAction);
    };
  }

  // Attach Copy button listeners
  container.querySelectorAll(".copy-btn").forEach((btn) => {
    (btn as HTMLButtonElement).onclick = async (e) => {
      const target = e.currentTarget as HTMLButtonElement;
      const text = decodeURIComponent(target.getAttribute("data-text") || "");
      if (text) {
        await navigator.clipboard.writeText(text);
        const originalText = target.textContent;
        target.textContent = "Copied!";
        target.style.background = "#16a34a";
        setTimeout(() => {
          target.textContent = originalText;
          target.style.background = "";
        }, 1500);
      }
    };
  });

  // Attach Re-paste button listeners
  container.querySelectorAll(".repaste-btn").forEach((btn) => {
    (btn as HTMLButtonElement).onclick = async (e) => {
      const target = e.currentTarget as HTMLButtonElement;
      const text = decodeURIComponent(target.getAttribute("data-text") || "");
      if (text) {
        target.disabled = true;
        target.textContent = "Pasting...";
        try {
          await CadenceIPC.paste(text);
          if (onRepasteAction) onRepasteAction(text);
        } catch (err: any) {
          console.error("Re-paste error:", err);
        } finally {
          target.disabled = false;
          target.textContent = "Re-paste";
        }
      }
    };
  });
}
