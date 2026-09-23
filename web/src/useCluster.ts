import { useEffect, useState } from "react";
import type { Snapshot } from "./types";

export function useCluster() {
  const [state, setState] = useState<Snapshot | null>(null);
  const [connection, setConnection] = useState<
    "connecting" | "live" | "reconnecting"
  >("connecting");
  const [error, setError] = useState("");
  useEffect(() => {
    let disposed = false;
    let socket: WebSocket;
    let retry: ReturnType<typeof setTimeout>;
    let attempts = 0;
    const connect = () => {
      socket = new WebSocket(
        `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api/ws`,
      );
      socket.onmessage = (event) => {
        if (disposed) return;
        try {
          const message = JSON.parse(event.data);
          if (message.type === "snapshot") {
            setState(message.data);
            setConnection("live");
            setError("");
            attempts = 0;
          } else if (message.type === "error") {
            setError(message.message);
            setConnection("reconnecting");
          }
        } catch {
          setError("The server sent an unreadable update.");
          setConnection("reconnecting");
        }
      };
      socket.onclose = () => {
        if (disposed) return;
        setConnection("reconnecting");
        retry = setTimeout(connect, Math.min(1000 * 2 ** attempts++, 10000));
      };
      socket.onerror = () => socket.close();
    };
    connect();
    return () => {
      disposed = true;
      clearTimeout(retry);
      socket.close();
    };
  }, []);
  return { state, connection, error };
}
