import * as net from 'net';
import * as os from 'os';
import * as path from 'path';

export interface EditorContextPayload {
  editor_name: string;
  workspace_root?: string;
  file_path?: string;
  file_name?: string;
  language_id?: string;
  line: number;
  column: number;
  selection_text?: string;
  selection_range?: {
    start_line: number;
    start_col: number;
    end_line: number;
    end_col: number;
  };
  surrounding_context?: string;
}

export class CadenceClient {
  private customSocketPath: string;

  constructor(customSocketPath = '') {
    this.customSocketPath = customSocketPath;
  }

  public setSocketPath(p: string) {
    this.customSocketPath = p;
  }

  public getSocketPath(): string {
    if (this.customSocketPath && this.customSocketPath.trim().length > 0) {
      return this.customSocketPath;
    }
    if (process.platform === 'win32') {
      return '\\\\.\\pipe\\cadence.sock';
    }
    const home = os.homedir();
    return path.join(home, '.cadence', 'cadence.sock');
  }

  public async call<T = unknown>(method: string, params: unknown = {}): Promise<T> {
    const socketPath = this.getSocketPath();

    return new Promise<T>((resolve, reject) => {
      const socket = net.createConnection({ path: socketPath });
      let responseBuffer = '';
      let isSettled = false;

      const finishError = (err: Error) => {
        if (!isSettled) {
          isSettled = true;
          socket.destroy();
          reject(err);
        }
      };

      const finishSuccess = (data: T) => {
        if (!isSettled) {
          isSettled = true;
          socket.destroy();
          resolve(data);
        }
      };

      socket.setTimeout(4000, () => {
        finishError(new Error(`Timeout calling Cadence IPC method: ${method}`));
      });

      socket.on('error', (err) => {
        finishError(new Error(`Cadence socket error: ${err.message}`));
      });

      socket.on('data', (chunk) => {
        responseBuffer += chunk.toString('utf-8');
        const newlineIdx = responseBuffer.indexOf('\n');
        if (newlineIdx !== -1) {
          const line = responseBuffer.slice(0, newlineIdx).trim();
          try {
            const parsed = JSON.parse(line);
            if (parsed.error) {
              finishError(new Error(`Cadence daemon error: ${parsed.error.message || JSON.stringify(parsed.error)}`));
            } else {
              finishSuccess(parsed.result as T);
            }
          } catch (e) {
            finishError(new Error(`Failed to parse Cadence response: ${e instanceof Error ? e.message : String(e)}`));
          }
        }
      });

      socket.on('connect', () => {
        const payload = JSON.stringify({
          jsonrpc: '2.0',
          id: String(Date.now()),
          method,
          params,
        }) + '\n';
        socket.write(payload, 'utf-8');
      });
    });
  }

  public async ping(): Promise<boolean> {
    try {
      await this.call('ping');
      return true;
    } catch {
      return false;
    }
  }

  public async status(): Promise<any> {
    return this.call('status');
  }

  public async setEditorContext(ctx: EditorContextPayload): Promise<any> {
    return this.call('editor_context_set', ctx);
  }

  public async clearEditorContext(): Promise<any> {
    return this.call('editor_context_clear');
  }

  public async executeEdit(instruction: string, text?: string, dryRun = false): Promise<any> {
    return this.call('edit_execute', {
      instruction,
      text,
      dry_run: dryRun,
    });
  }

  public async undo(depth = 1): Promise<any> {
    return this.call('undo', { depth });
  }

  public async toggleDictation(): Promise<any> {
    const status: any = await this.status();
    if (status && status.recording) {
      return this.call('dictate_stop');
    } else {
      return this.call('dictate_start');
    }
  }
}
