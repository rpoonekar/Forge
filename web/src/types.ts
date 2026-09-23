export type TaskStatus =
  | "BLOCKED"
  | "QUEUED"
  | "RUNNING"
  | "RETRYING"
  | "SUCCEEDED"
  | "FAILED"
  | "CANCELED";
export interface Task {
  ID: string;
  BuildID: string;
  Name: string;
  Command: string;
  Status: TaskStatus;
  WorkerID: string;
  CreatedAt: string;
  StartedAt: string;
  EndedAt: string;
  Output: string;
  ExitCode: number;
  RetryCount: number;
  MaxRetries: number;
  NextRetryAt: string;
}
export interface Build {
  id: string;
  status: string;
  created_at: string;
  ended_at: string;
  tasks?: Task[];
}
export interface Worker {
  ID: string;
  Status: string;
  CurrentTask: string;
  LastSeen: string;
  TasksRun: number;
}
export interface Snapshot {
  builds: Build[];
  workers: Worker[];
  dependencies: Record<string, string[]>;
  captured_at: string;
  metrics: {
    queue_depth: number;
    running: number;
    throughput: number;
    retries: number;
    scheduling_latency_ms: number | null;
  };
  demo_enabled: boolean;
  demo_workers: string[];
  stopping_workers: string[];
}
export interface TaskSpec {
  name: string;
  command: string;
  depends_on: string[];
}
