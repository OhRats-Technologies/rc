import { Elysia } from "elysia";
import { cookieUser } from "../auth";
import { subscribeEvents } from "../events";
import { fail } from "../http-utils";

export const eventRoutes = new Elysia({ name: "rc.events.http", prefix: "/api/v1" })
  .get("/events", async ({ request }) => {
    if (request.headers.has("authorization") || request.headers.has("x-rc-key-id")) return fail("browser session required", 401);
    const user = await cookieUser(request); if (!user) return fail("authentication required", 401);
    const encoder = new TextEncoder();
    let unsubscribe = () => {}, heartbeat: ReturnType<typeof setInterval> | null = null, closed = false;
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        const send = (value: string) => { if (!closed) controller.enqueue(encoder.encode(value)); };
        send("retry: 1000\n\n");
        unsubscribe = subscribeEvents(user.id, event => send(`data: ${JSON.stringify(event)}\n\n`));
        heartbeat = setInterval(() => send(": keepalive\n\n"), 15_000); heartbeat.unref?.();
        const close = () => {
          if (closed) return; closed = true; unsubscribe(); if (heartbeat) clearInterval(heartbeat);
          try { controller.close(); } catch {}
        };
        request.signal.addEventListener("abort", close, { once: true });
      },
      cancel() { closed = true; unsubscribe(); if (heartbeat) clearInterval(heartbeat); },
    });
    return new Response(stream, { headers: {
      "content-type": "text/event-stream; charset=utf-8", "cache-control": "no-cache, no-store",
      "connection": "keep-alive", "x-accel-buffering": "no",
    } });
  }, { detail: { hide: true } });
