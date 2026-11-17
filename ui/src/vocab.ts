import { CadenceIPC, type CadenceVocabularyItem } from "./ipc";

export interface VocabState {
  items: CadenceVocabularyItem[];
  searchQuery: string;
  loading: boolean;
  statusMessage: string;
}

const state: VocabState = {
  items: [
    { spoken: "cube control", canonical: "kubectl" },
    { spoken: "pie torch", canonical: "PyTorch" },
    { spoken: "post gres", canonical: "PostgreSQL" },
    { spoken: "cadence", canonical: "Cadence" },
    { spoken: "taw ri", canonical: "Tauri" },
    { spoken: "github", canonical: "GitHub" },
  ],
  searchQuery: "",
  loading: false,
  statusMessage: "",
};

export async function fetchVocabulary(): Promise<CadenceVocabularyItem[]> {
  state.loading = true;
  try {
    const list = await CadenceIPC.getVocabulary();
    if (list && list.length > 0) {
      state.items = list;
    }
  } catch (err) {
    console.warn("Failed to fetch vocabulary via IPC:", err);
  } finally {
    state.loading = false;
  }
  return state.items;
}

export function renderVocabularyManager(container: HTMLElement): void {
  const q = state.searchQuery.trim().toLowerCase();
  const filtered = state.items.filter(
    (item) =>
      !q ||
      item.spoken.toLowerCase().includes(q) ||
      item.canonical.toLowerCase().includes(q)
  );

  container.innerHTML = `
    <div class="vocab-panel">
      <div class="vocab-header">
        <div>
          <h3>Custom Vocabulary Manager</h3>
          <p class="vocab-subtitle">Define domain terminology, technical abbreviations, and acronyms for instant phonetic replacement.</p>
        </div>
        <div class="vocab-actions-top">
          <button id="btn-export-vocab" class="action-btn" style="background: #475569;">Export JSON</button>
          <button id="btn-refresh-vocab" class="action-btn" style="background: #3b82f6;">Refresh</button>
        </div>
      </div>

      ${state.statusMessage ? `<div class="save-banner">${state.statusMessage}</div>` : ""}

      <!-- Add New Word Form -->
      <div class="vocab-add-card">
        <h4 style="margin-bottom: 10px; color: #1e293b;">Add Custom Term</h4>
        <form id="vocab-add-form" class="vocab-add-form">
          <div class="vocab-input-group">
            <label for="vocab-spoken">Spoken Phrase (how you say it)</label>
            <input id="vocab-spoken" type="text" class="form-input" placeholder="e.g. cube control" required />
          </div>
          <div class="vocab-input-group">
            <label for="vocab-canonical">Canonical Replacement (how it writes)</label>
            <input id="vocab-canonical" type="text" class="form-input" placeholder="e.g. kubectl" required />
          </div>
          <button type="submit" class="action-btn" style="background: #16a34a; align-self: flex-end; padding: 7px 18px;">Add Term</button>
        </form>
      </div>

      <!-- Search & Vocabulary Table -->
      <div class="vocab-table-container">
        <div class="vocab-table-toolbar">
          <input
            id="vocab-search"
            type="search"
            class="history-search"
            placeholder="Search vocabulary..."
            value="${state.searchQuery}"
            style="width: 280px;"
          />
          <span style="font-size: 12px; color: #64748b; margin-left: auto;">${filtered.length} term(s)</span>
        </div>

        <table class="vocab-table">
          <thead>
            <tr>
              <th style="width: 45%;">Spoken Phrase</th>
              <th style="width: 45%;">Canonical Replacement</th>
              <th style="width: 10%; text-align: right;">Action</th>
            </tr>
          </thead>
          <tbody>
            ${
              filtered.length === 0
                ? `<tr><td colspan="3" style="text-align: center; color: #64748b; padding: 24px;">No vocabulary terms found.</td></tr>`
                : filtered
                    .map(
                      (item) => `
                <tr>
                  <td class="vocab-spoken-term">"${item.spoken}"</td>
                  <td class="vocab-canonical-term"><code>${item.canonical}</code></td>
                  <td style="text-align: right;">
                    <button class="action-btn delete-vocab-btn" data-spoken="${encodeURIComponent(item.spoken)}" style="background: #ef4444; font-size: 11px; padding: 2px 8px;">Delete</button>
                  </td>
                </tr>
              `
                    )
                    .join("")
            }
          </tbody>
        </table>
      </div>
    </div>
  `;

  // Search input listener
  const searchInput = container.querySelector("#vocab-search") as HTMLInputElement | null;
  if (searchInput) {
    searchInput.oninput = () => {
      state.searchQuery = searchInput.value;
      renderVocabularyManager(container);
    };
  }

  // Add Form listener
  const form = container.querySelector("#vocab-add-form") as HTMLFormElement | null;
  if (form) {
    form.onsubmit = async (e) => {
      e.preventDefault();
      const spokenInput = container.querySelector("#vocab-spoken") as HTMLInputElement;
      const canonicalInput = container.querySelector("#vocab-canonical") as HTMLInputElement;
      const spoken = spokenInput.value.trim();
      const canonical = canonicalInput.value.trim();

      if (!spoken || !canonical) return;

      try {
        await CadenceIPC.addVocabulary(spoken, canonical);
        state.statusMessage = `Added term: "${spoken}" → "${canonical}"`;
        // Optimistically update local cache
        state.items = state.items.filter((i) => i.spoken.toLowerCase() !== spoken.toLowerCase());
        state.items.unshift({ spoken, canonical });
      } catch (err: any) {
        state.statusMessage = `Failed to add term: ${err?.message || err}`;
      } finally {
        renderVocabularyManager(container);
      }
    };
  }

  // Delete button listeners
  container.querySelectorAll(".delete-vocab-btn").forEach((btn) => {
    (btn as HTMLButtonElement).onclick = async (e) => {
      const target = e.currentTarget as HTMLButtonElement;
      const spoken = decodeURIComponent(target.getAttribute("data-spoken") || "");
      if (!spoken) return;

      try {
        await CadenceIPC.deleteVocabulary(spoken);
        state.statusMessage = `Deleted term: "${spoken}"`;
        state.items = state.items.filter((i) => i.spoken !== spoken);
      } catch (err: any) {
        state.statusMessage = `Failed to delete: ${err?.message || err}`;
      } finally {
        renderVocabularyManager(container);
      }
    };
  });

  // Export JSON button listener
  const exportBtn = container.querySelector("#btn-export-vocab") as HTMLButtonElement | null;
  if (exportBtn) {
    exportBtn.onclick = async () => {
      try {
        const jsonStr = await CadenceIPC.exportVocabulary("json");
        const blob = new Blob([jsonStr], { type: "application/json" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `cadence_vocabulary_${Date.now()}.json`;
        a.click();
        URL.revokeObjectURL(url);
        state.statusMessage = "Vocabulary exported successfully!";
      } catch (err: any) {
        state.statusMessage = `Export error: ${err?.message || err}`;
      } finally {
        renderVocabularyManager(container);
      }
    };
  }

  // Refresh button listener
  const refreshBtn = container.querySelector("#btn-refresh-vocab") as HTMLButtonElement | null;
  if (refreshBtn) {
    refreshBtn.onclick = async () => {
      refreshBtn.disabled = true;
      refreshBtn.textContent = "Loading...";
      await fetchVocabulary();
      state.statusMessage = "Vocabulary refreshed from daemon.";
      renderVocabularyManager(container);
    };
  }
}
