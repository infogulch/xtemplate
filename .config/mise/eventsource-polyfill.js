// EventSource polyfill for lightpanda, which does not implement EventSource.
// Uses fetch + ReadableStream (both supported) to replicate the SSE protocol.
class EventSource {
    constructor(url) {
        this.onmessage = null;
        const u = new URL(url, location.href);
        fetch(u, { headers: { Accept: "text/event-stream" } })
            .then(async (r) => {
                const reader = r.body.getReader();
                const dec = new TextDecoder();
                let buf = "";
                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    buf += dec.decode(value);
                    const lines = buf.split("\n");
                    buf = lines.pop(); // hold back any incomplete trailing line
                    for (const line of lines) {
                        if (line.startsWith("data:") && this.onmessage) {
                            this.onmessage({ data: line.slice(5).trim() });
                        }
                    }
                }
            })
            .catch((e) => console.error("eventsource-polyfill:", String(e)));
    }
    close() {}
}
