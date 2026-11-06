import * as vscode from 'vscode';
import * as path from 'path';
import { CadenceClient, EditorContextPayload } from './client';

let client: CadenceClient;
let statusBarItem: vscode.StatusBarItem;
let syncDebounceTimer: NodeJS.Timeout | undefined;
let heartbeatTimer: NodeJS.Timeout | undefined;

export function activate(context: vscode.ExtensionContext) {
  const config = vscode.workspace.getConfiguration('cadence');
  const customSocket = config.get<string>('socketPath') || '';

  client = new CadenceClient(customSocket);

  // Status Bar Item
  statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
  statusBarItem.command = 'cadence.status';
  statusBarItem.text = '$(mic) Cadence';
  statusBarItem.tooltip = 'Cadence Voice Engine';
  statusBarItem.show();
  context.subscriptions.push(statusBarItem);

  // Refresh status bar
  updateStatusBar();
  heartbeatTimer = setInterval(updateStatusBar, 10000);

  // Sync on editor switch / selection change
  context.subscriptions.push(
    vscode.window.onDidChangeActiveTextEditor(() => scheduleSyncContext()),
    vscode.window.onDidChangeTextEditorSelection(() => scheduleSyncContext()),
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration('cadence.socketPath')) {
        const newSocket = vscode.workspace.getConfiguration('cadence').get<string>('socketPath') || '';
        client.setSocketPath(newSocket);
        updateStatusBar();
      }
    })
  );

  // Commands
  context.subscriptions.push(
    vscode.commands.registerCommand('cadence.syncContext', async () => {
      await syncCurrentEditorContext();
      vscode.window.showInformationMessage('Cadence: Editor context synced.');
    }),

    vscode.commands.registerCommand('cadence.status', async () => {
      try {
        const st: any = await client.status();
        const stateMsg = st.recording ? 'RECORDING' : 'READY';
        const model = st.model || 'local';
        vscode.window.showInformationMessage(
          `Cadence Daemon (${st.version || 'v0.6.0'})\nStatus: ${stateMsg}\nModel: ${model}\nSocket: ${client.getSocketPath()}`
        );
      } catch (err: any) {
        vscode.window.showErrorMessage(`Cadence Daemon offline: ${err.message}`);
      }
    }),

    vscode.commands.registerCommand('cadence.dictate', async () => {
      try {
        await syncCurrentEditorContext();
        await client.toggleDictation();
        updateStatusBar();
      } catch (err: any) {
        vscode.window.showErrorMessage(`Cadence Dictate error: ${err.message}`);
      }
    }),

    vscode.commands.registerCommand('cadence.editSelection', async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        vscode.window.showWarningMessage('No active editor open.');
        return;
      }

      const selection = editor.selection;
      let targetText = editor.document.getText(selection);
      if (!targetText || targetText.trim().length === 0) {
        // Fallback to active line if nothing explicitly selected
        const currentLine = editor.document.lineAt(selection.active.line);
        targetText = currentLine.text;
      }

      if (!targetText || targetText.trim().length === 0) {
        vscode.window.showWarningMessage('No text selected or available on current line to edit.');
        return;
      }

      const instruction = await vscode.window.showInputBox({
        prompt: 'Enter edit instruction (e.g. "rewrite as async", "convert into bullet list", "add error check")',
        placeHolder: 'Voice or text instruction...',
      });

      if (!instruction || instruction.trim().length === 0) {
        return;
      }

      // Sync active context right before sending edit
      await syncCurrentEditorContext();

      vscode.window.withProgress(
        {
          location: vscode.ProgressLocation.Notification,
          title: `Cadence: Transforming code with "${instruction}"...`,
          cancellable: false,
        },
        async () => {
          try {
            const res: any = await client.executeEdit(instruction, targetText, true);
            if (res && res.edited_text) {
              const rangeToReplace = selection.isEmpty
                ? editor.document.lineAt(selection.active.line).range
                : selection;

              await editor.edit((editBuilder) => {
                editBuilder.replace(rangeToReplace, res.edited_text);
              });

              vscode.window.showInformationMessage(
                `Cadence: Edit applied (${res.latency_ms || 0}ms)`
              );
            }
          } catch (err: any) {
            vscode.window.showErrorMessage(`Cadence Edit failed: ${err.message}`);
          }
        }
      );
    }),

    vscode.commands.registerCommand('cadence.undo', async () => {
      try {
        const res: any = await client.undo(1);
        vscode.window.showInformationMessage(`Cadence: Undo applied (Remaining depth: ${res.remaining ?? 0})`);
      } catch (err: any) {
        vscode.window.showErrorMessage(`Cadence Undo error: ${err.message}`);
      }
    })
  );

  // Initial sync
  scheduleSyncContext();
}

export function deactivate() {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer);
    heartbeatTimer = undefined;
  }
  if (syncDebounceTimer) {
    clearTimeout(syncDebounceTimer);
    syncDebounceTimer = undefined;
  }
  if (client) {
    client.clearEditorContext().catch(() => {});
  }
}

async function updateStatusBar() {
  if (!statusBarItem || !client) {
    return;
  }
  try {
    const isOnline = await client.ping();
    if (isOnline) {
      statusBarItem.text = '$(mic) Cadence $(check)';
      statusBarItem.tooltip = `Cadence Daemon connected at ${client.getSocketPath()}`;
      statusBarItem.color = undefined;
    } else {
      statusBarItem.text = '$(mic) Cadence $(circle-slash)';
      statusBarItem.tooltip = 'Cadence Daemon offline. Run `cadenced` to start.';
      statusBarItem.color = new vscode.ThemeColor('errorForeground');
    }
  } catch {
    statusBarItem.text = '$(mic) Cadence $(circle-slash)';
    statusBarItem.tooltip = 'Cadence Daemon unreachable.';
    statusBarItem.color = new vscode.ThemeColor('errorForeground');
  }
}

function scheduleSyncContext() {
  const config = vscode.workspace.getConfiguration('cadence');
  if (!config.get<boolean>('autoSync', true)) {
    return;
  }

  if (syncDebounceTimer) {
    clearTimeout(syncDebounceTimer);
  }

  syncDebounceTimer = setTimeout(() => {
    syncCurrentEditorContext().catch(() => {});
  }, 200);
}

async function syncCurrentEditorContext() {
  const editor = vscode.window.activeTextEditor;
  if (!editor || !client) {
    return;
  }

  const document = editor.document;
  const selection = editor.selection;
  const config = vscode.workspace.getConfiguration('cadence');
  const surroundingLines = config.get<number>('surroundingLines', 15);

  const totalLines = document.lineCount;
  const currentLine = selection.active.line;

  const startLine = Math.max(0, currentLine - surroundingLines);
  const endLine = Math.min(totalLines - 1, currentLine + surroundingLines);

  const surroundingRange = new vscode.Range(
    new vscode.Position(startLine, 0),
    new vscode.Position(endLine, document.lineAt(endLine).range.end.character)
  );

  const surroundingCode = document.getText(surroundingRange);
  const selectedText = document.getText(selection);

  const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
  const workspaceRoot = workspaceFolder ? workspaceFolder.uri.fsPath : '';

  const payload: EditorContextPayload = {
    editor_name: 'vscode',
    workspace_root: workspaceRoot,
    file_path: document.uri.fsPath,
    file_name: path.basename(document.uri.fsPath),
    language_id: document.languageId,
    line: currentLine + 1,
    column: selection.active.character + 1,
    selection_text: selectedText,
    selection_range: {
      start_line: selection.start.line + 1,
      start_col: selection.start.character + 1,
      end_line: selection.end.line + 1,
      end_col: selection.end.character + 1,
    },
    surrounding_context: surroundingCode,
  };

  try {
    await client.setEditorContext(payload);
  } catch {
    // Daemon might not be running; non-blocking
  }
}
