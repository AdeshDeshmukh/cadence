import { CadenceIPC, type CadenceSnippet } from "./ipc";

export interface SnippetsState {
  items: CadenceSnippet[];
  searchQuery: string;
  loading: boolean;
  statusMessage: string;
  testInput: string;
  testOutput: string;
  editingTrigger: string | null;
}

const state: SnippetsState = {
  items: [
    {
      trigger: "!date",
      template: "{{date}}",
      description: "Current ISO date (YYYY-MM-DD)",
    },
    {
      trigger: "!time",
      template: "{{time}}",
      description: "Current 24h time (HH:MM:SS)",
    },
    {
      trigger: "shrug",
      template: "¯\\_(ツ)_/¯",
      description: "ASCII shrug kaomoji",
    },
    {
      trigger: "!email",
      template: "Best regards,\n{{cursor}}",
      description: "Email sign-off template",
    },
    {
      trigger: "!todo",
      template: "- [ ] {{cursor}}",
      description: "Markdown task item",
    },
  ],
  searchQuery: "",
  loading: false,
  statusMessage: "",
  testInput: "{{date}} - Meeting notes: {{clipboard}}",
  testOutput: "",
  editingTrigger: null,
};

export async function fetchSnippets(): Promise<CadenceSnippet[]> {
  state.loading = true;
  try {
    const list = await CadenceIPC.getSnippets();
    if (list && Array.isArray(list) && list.length > 0) {
      state.items = list;
    }
  } catch (err) {
    console.warn("Failed to fetch snippets via IPC:", err);
  } finally {
    state.loading = false;
  }
  return state.items;
}

export function renderSnippetManager(container: HTMLElement): void {
  const q = state.searchQuery.trim().toLowerCase();
  const filtered = state.items.filter(
    (item) =>
      !q ||
      item.trigger.toLowerCase().includes(q) ||
      item.template.toLowerCase().includes(q) ||
      (item.description && item.description.toLowerCase().includes(q))
  );

  const editingItem = state.editingTrigger
    ? state.items.find((i) => i.trigger === state.editingTrigger)
    : null;

  container.innerHTML = `
    <div class="snippets-panel">
      <div class="vocab-header">
        <div>
          <h3>Voice & Text Snippet Manager</h3>
          <p class="vocab-subtitle">Create trigger phrases and dynamic templates expanded instantly during dictation.</p>
        </div>
        <div class="vocab-actions-top">
          <button id="btn-refresh-snippets" class="action-btn" style="background: #3b82f6;">Refresh</button>
        </div>
      </div>

      ${state.statusMessage ? `<div class="save-banner">${state.statusMessage}</div>` : ""}

      <!-- Add / Edit Snippet Form -->
      <div class="snippet-card">
        <h4 style="margin-bottom: 12px; color: #1e293b;">${editingItem ? "Edit Snippet" : "Create New Snippet"}</h4>
        <form id="snippet-form" class="snippet-form">
          <div style="display: flex; gap: 16px; flex-wrap: wrap;">
            <div class="vocab-input-group" style="flex: 1; min-width: 180px;">
              <label for="snip-trigger">Trigger Word / Phrase</label>
              <input
                id="snip-trigger"
                type="text"
                class="form-input"
                placeholder="e.g. !standup or shrug"
                value="${editingItem ? editingItem.trigger : ""}"
                ${editingItem ? "readonly style='opacity: 0.8;'" : "required"}
              />
            </div>
            <div class="vocab-input-group" style="flex: 2; min-width: 240px;">
              <label for="snip-desc">Description (Optional)</label>
              <input
                id="snip-desc"
                type="text"
                class="form-input"
                placeholder="e.g. Daily agile standup checklist"
                value="${editingItem && editingItem.description ? editingItem.description : ""}"
              />
            </div>
          </div>

          <div class="vocab-input-group" style="margin-top: 10px;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
              <label for="snip-template">Template Content</label>
              <div class="variable-pills">
                <span class="var-pill" data-var="{{date}}">+ {{date}}</span>
                <span class="var-pill" data-var="{{time}}">+ {{time}}</span>
                <span class="var-pill" data-var="{{clipboard}}">+ {{clipboard}}</span>
                <span class="var-pill" data-var="{{cursor}}">+ {{cursor}}</span>
              </div>
            </div>
            <textarea
              id="snip-template"
              class="form-input"
              rows="3"
              placeholder="e.g. ¯\\_(ツ)_/¯ or Date: {{date}}\nNotes: {{clipboard}}"
              required
            >${editingItem ? editingItem.template : ""}</textarea>
          </div>

          <div style="display: flex; gap: 8px; justify-content: flex-end; margin-top: 12px;">
            ${
              editingItem
                ? `<button type="button" id="btn-cancel-edit" class="action-btn" style="background: #64748b;">Cancel</button>`
                : ""
            }
            <button type="submit" class="action-btn" style="background: #16a34a;">
              ${editingItem ? "Update Snippet" : "Save Snippet"}
            </button>
          </div>
        </form>
      </div>

      <!-- Live Snippet Expansion Tester -->
      <div class="snippet-tester-card">
        <h4 style="margin-bottom: 8px; color: #1e293b;">Live Snippet Expansion Tester</h4>
        <div style="display: flex; gap: 12px; align-items: center; flex-wrap: wrap;">
          <input
            id="tester-input"
            type="text"
            class="form-input"
            style="flex: 1; min-width: 250px;"
            placeholder="Type snippet trigger or template e.g. !date or Hello {{date}}"
            value="${state.testInput}"
          />
          <button id="btn-run-test" class="action-btn" style="background: #8b5cf6;">Test Expand</button>
        </div>
        ${
          state.testOutput
            ? `
          <div class="test-output-box">
            <span style="font-size: 11px; font-weight: 600; color: #64748b; text-transform: uppercase;">Expanded Output:</span>
            <pre class="test-output-pre">${state.testOutput}</pre>
          </div>
        `
            : ""
        }
      </div>

      <!-- Snippets Table -->
      <div class="vocab-table-container">
        <div class="vocab-table-toolbar">
          <input
            id="snippet-search"
            type="search"
            class="history-search"
            placeholder="Search snippets..."
            value="${state.searchQuery}"
            style="width: 280px;"
          />
          <span style="font-size: 12px; color: #64748b; margin-left: auto;">${filtered.length} snippet(s)</span>
        </div>

        <table class="vocab-table">
          <thead>
            <tr>
              <th style="width: 20%;">Trigger</th>
              <th style="width: 45%;">Template</th>
              <th style="width: 20%;">Description</th>
              <th style="width: 15%; text-align: right;">Actions</th>
            </tr>
          </thead>
          <tbody>
            ${
              filtered.length === 0
                ? `<tr><td colspan="4" style="text-align: center; color: #64748b; padding: 24px;">No snippets found.</td></tr>`
                : filtered
                    .map(
                      (item) => `
                <tr>
                  <td class="vocab-spoken-term"><span class="badge" style="background: rgba(99, 102, 241, 0.15); color: #6366f1;">${item.trigger}</span></td>
                  <td class="snippet-template-cell"><pre class="snippet-pre">${escapeHtml(item.template)}</pre></td>
                  <td style="color: #64748b; font-size: 12px;">${escapeHtml(item.description || "")}</td>
                  <td style="text-align: right; white-space: nowrap;">
                    <button class="action-btn test-snip-btn" data-trigger="${encodeURIComponent(item.trigger)}" data-template="${encodeURIComponent(item.template)}" style="background: #8b5cf6; font-size: 11px; padding: 2px 7px;">Test</button>
                    <button class="action-btn edit-snip-btn" data-trigger="${encodeURIComponent(item.trigger)}" style="background: #3b82f6; font-size: 11px; padding: 2px 7px;">Edit</button>
                    <button class="action-btn delete-snip-btn" data-trigger="${encodeURIComponent(item.trigger)}" style="background: #ef4444; font-size: 11px; padding: 2px 7px;">Delete</button>
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

  // Attach search listener
  const searchInput = container.querySelector("#snippet-search") as HTMLInputElement | null;
  if (searchInput) {
    searchInput.oninput = () => {
      state.searchQuery = searchInput.value;
      renderSnippetManager(container);
    };
  }

  // Variable pills click to insert at cursor in textarea
  const textarea = container.querySelector("#snip-template") as HTMLTextAreaElement | null;
  container.querySelectorAll(".var-pill").forEach((pill) => {
    (pill as HTMLElement).onclick = () => {
      const varTag = pill.getAttribute("data-var") || "";
      if (!textarea) return;
      const start = textarea.selectionStart || 0;
      const end = textarea.selectionEnd || 0;
      const before = textarea.value.substring(0, start);
      const after = textarea.value.substring(end);
      textarea.value = before + varTag + after;
      textarea.selectionStart = textarea.selectionEnd = start + varTag.length;
      textarea.focus();
    };
  });

  // Snippet Form submission
  const form = container.querySelector("#snippet-form") as HTMLFormElement | null;
  if (form) {
    form.onsubmit = async (e) => {
      e.preventDefault();
      const triggerInput = container.querySelector("#snip-trigger") as HTMLInputElement;
      const descInput = container.querySelector("#snip-desc") as HTMLInputElement;
      const templateInput = container.querySelector("#snip-template") as HTMLTextAreaElement;

      const trigger = triggerInput.value.trim();
      const description = descInput.value.trim();
      const template = templateInput.value;

      if (!trigger || !template) return;

      try {
        await CadenceIPC.setSnippet(trigger, template, description);
        state.statusMessage = `Saved snippet: "${trigger}"`;
        // Optimistically update list
        state.items = state.items.filter((s) => s.trigger !== trigger);
        state.items.unshift({ trigger, template, description });
        state.editingTrigger = null;
      } catch (err: any) {
        state.statusMessage = `Failed to save snippet: ${err?.message || err}`;
      } finally {
        renderSnippetManager(container);
      }
    };
  }

  // Cancel edit button
  const cancelBtn = container.querySelector("#btn-cancel-edit") as HTMLButtonElement | null;
  if (cancelBtn) {
    cancelBtn.onclick = () => {
      state.editingTrigger = null;
      renderSnippetManager(container);
    };
  }

  // Live Tester
  const runTestBtn = container.querySelector("#btn-run-test") as HTMLButtonElement | null;
  const testInputEl = container.querySelector("#tester-input") as HTMLInputElement | null;
  if (runTestBtn && testInputEl) {
    runTestBtn.onclick = async () => {
      const input = testInputEl.value.trim();
      state.testInput = input;
      if (!input) return;

      try {
        // Expand either by trigger match or as a raw template string
        const match = state.items.find((s) => s.trigger === input);
        const res = await CadenceIPC.expandSnippet(
          match ? match.template : input,
          match ? match.trigger : undefined
        );
        state.testOutput = res?.expanded || "(no expansion produced)";
      } catch (err: any) {
        // Fallback local substitution if daemon offline
        state.testOutput = localExpandFallback(input);
      } finally {
        renderSnippetManager(container);
      }
    };
  }

  // Table action listeners
  container.querySelectorAll(".test-snip-btn").forEach((btn) => {
    (btn as HTMLButtonElement).onclick = async (e) => {
      const target = e.currentTarget as HTMLButtonElement;
      const tmpl = decodeURIComponent(target.getAttribute("data-template") || "");
      const trig = decodeURIComponent(target.getAttribute("data-trigger") || "");
      state.testInput = trig;
      try {
        const res = await CadenceIPC.expandSnippet(tmpl, trig);
        state.testOutput = res?.expanded || tmpl;
      } catch {
        state.testOutput = localExpandFallback(tmpl);
      }
      renderSnippetManager(container);
    };
  });

  container.querySelectorAll(".edit-snip-btn").forEach((btn) => {
    (btn as HTMLButtonElement).onclick = (e) => {
      const target = e.currentTarget as HTMLButtonElement;
      const trig = decodeURIComponent(target.getAttribute("data-trigger") || "");
      state.editingTrigger = trig;
      renderSnippetManager(container);
      container.querySelector("#snip-template")?.scrollIntoView({ behavior: "smooth" });
    };
  });

  container.querySelectorAll(".delete-snip-btn").forEach((btn) => {
    (btn as HTMLButtonElement).onclick = async (e) => {
      const target = e.currentTarget as HTMLButtonElement;
      const trig = decodeURIComponent(target.getAttribute("data-trigger") || "");
      if (!trig) return;

      try {
        await CadenceIPC.deleteSnippet(trig);
        state.statusMessage = `Deleted snippet: "${trig}"`;
        state.items = state.items.filter((s) => s.trigger !== trig);
        if (state.editingTrigger === trig) state.editingTrigger = null;
      } catch (err: any) {
        state.statusMessage = `Failed to delete snippet: ${err?.message || err}`;
      } finally {
        renderSnippetManager(container);
      }
    };
  });

  // Refresh button
  const refreshBtn = container.querySelector("#btn-refresh-snippets") as HTMLButtonElement | null;
  if (refreshBtn) {
    refreshBtn.onclick = async () => {
      refreshBtn.disabled = true;
      refreshBtn.textContent = "Loading...";
      await fetchSnippets();
      state.statusMessage = "Snippets refreshed from daemon.";
      renderSnippetManager(container);
    };
  }
}

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function localExpandFallback(template: string): string {
  const now = new Date();
  const dateStr = now.toISOString().split("T")[0];
  const timeStr = now.toTimeString().split(" ")[0];
  return template
    .replace(/\{\{date\}\}/g, dateStr)
    .replace(/\{\{time\}\}/g, timeStr)
    .replace(/\{\{clipboard\}\}/g, "[Clipboard Content]")
    .replace(/\{\{cursor\}\}/g, "|");
}
