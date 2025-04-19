const DISPLAY_LIMIT = 10000;

const GENERIC_ERROR_MESSAGE = "An error occurred and the request could not be completed.";
const RATE_LIMITED_ERROR_MESSAGE = "Your IP has made too many requests lately. Please try again later.";

async function getManifest() {
    const httpResult = await fetch("./manifest");
    if (httpResult.ok) {
        return await httpResult.json();
    } else {
        console.error(`Failed to fetch manifest (HTTP ${httpResult.status}).`)
    }
}

async function search(keyword, resultsOut, statsOut) {
    const startTime = performance.now();

    const controller = new AbortController();
    const httpResult = await fetch(`./concord?w=${encodeURIComponent(keyword)}`, { signal: controller.signal });
    if (!httpResult.ok) {
        if (httpResult.status === 429) {
            throw { error: { message: RATE_LIMITED_ERROR_MESSAGE } };
        } else {
            const data = await httpResult.json();
            throw { error: data.error };
        }
    }

    const reader = httpResult.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
        let { done, value } = await reader.read();
        if (!done) {
            buffer += decoder.decode(value, { stream: true });
        }
        const lines = buffer.split("\n");
        // If `buffer` ends with a newline (or is empty), last item of `lines` will be the empty string.
        // Otherwise, it will have a half-completed line which we put back on the buffer.
        buffer = lines.pop();
        for (const line of lines) {
            const trimmed = line.trim();
            if (trimmed) {
                let data;
                try {
                    data = JSON.parse(trimmed);
                } catch (e) {
                    console.error("error decoding JSON", e, trimmed);
                    return;
                }

                if (data.status !== undefined) {
                    if (data.status === "queued") {
                        statsOut.queued = true;
                    } else if (data.status === "ready") {
                        statsOut.queued = false;
                    } else {
                        console.warn("Unknown status message received from server:", data);
                    }
                } else {
                    statsOut.queued = false;
                    resultsOut.push(data);
                    if (statsOut.millisToFirstResult === null) {
                        statsOut.millisToFirstResult = performance.now() - startTime;
                    }
                }
            }
        }

        if (resultsOut.length >= DISPLAY_LIMIT) {
            console.log(`Number of results exceeded DISPLAY_LIMIT (${DISPLAY_LIMIT}). Aborting request.`)
            controller.abort();
            done = true;
        }

        if (done) {
            statsOut.millisToLastResult = performance.now() - startTime;
            if (statsOut.millisToFirstResult === null) {
                // in case we got 0 results
                statsOut.millisToFirstResult = statsOut.millisToLastResult;
            }
            m.redraw();
            break;
        } else {
            m.redraw();
        }
    }
}

class PageView {
    constructor() {
        this.keyword = "";
        this.results = [];
        this.stats = { millisToFirstResult: null, millisToLastResult: null, queued: false };
        this.error = null;
        this.loading = false;
        this.manifest = null;

        getManifest().then(m => {
            this.manifest = m;
        });
        // ignore error: if we can't load manifest we can still load results
    }

    view() {
        const showError = this.error !== null;
        const showQueued = !showError && this.stats.queued;
        const showResults = !showQueued;
        const showLoading = showResults && this.loading;
        return m("main", [
            m(InputView, { onEnter: (keyword) => this.onEnter(keyword) }),
            m(StatsView, { stats: this.stats, resultsCount: this.results.length }),
            showError ? m(ErrorView, { error: this.error }) : null,
            showQueued ? m(QueuedView) : null,
            showResults ? m(ResultsListView, { keyword: this.keyword, results: this.results, manifest: this.manifest }) : null,
            showLoading ? m(LoadingView) : null,
        ]);
    }

    onEnter(keyword) {
        this.keyword = keyword;
        this.results = [];
        this.stats.millisToFirstResult = null;
        this.stats.millisToLastResult = null;
        this.stats.queued = false;
        this.error = null;
        this.loading = true;
        search(this.keyword, this.results, this.stats).then(() => {
            this.loading = false;
            m.redraw();
        }).catch((e) => {
            console.error("Error encountered in search().", e);
            this.loading = false;
            if (e.error !== undefined && typeof e.error.message === "string") {
                this.error = e.error.message;
            } else {
                this.error = GENERIC_ERROR_MESSAGE;
            }
            m.redraw();
        });
    }
}

class ErrorView {
    view(vnode) {
        const error = vnode.attrs.error;
        return m("div.error", [error]);
    }
}

class LoadingView {
    view() {
        return m("div.message", "Loading more results...");
    }
}

class QueuedView {
    view() {
        return m("div.message", "The server is under heavy load. Your request has been queued, and will begin shortly.");
    }
}

class InputView {
    constructor(vnode) {
        this.value = "";
        this.onEnter = vnode.attrs.onEnter;
    }

    view() {
        return m("input", { autocapitalize: "off", placeholder: "Enter a keyword (try 'vampire')", onkeydown: (e) => this.onkeydown(e) });
    }

    onkeydown(e) {
        if (e.keyCode === 13) {
            const keyword = e.target.value;
            e.target.value = "";
            // hides keyboard on mobile
            e.target.blur();
            this.onEnter(keyword);
        }
    }
}

class StatsView {
    view(vnode) {
        // TODO: more stats: books searched, books matched, bytes searched?
        const stats = vnode.attrs.stats;
        const resultsCount = vnode.attrs.resultsCount;
        if (stats.millisToFirstResult === null) {
            return null;
        }

        const s = resultsCount === 1 ? "" : "s";
        const firstMs = stats.millisToFirstResult.toFixed(1);
        let doneAfter = "";
        if (stats.millisToLastResult !== null && stats.millisToLastResult !== stats.millisToFirstResult) {
            const lastMs = stats.millisToLastResult.toFixed(1);
            doneAfter = `(done after ${lastMs}ms)`;
        }
        return m("div.stats",
            `${resultsCount} result${s} in ${firstMs}ms ${doneAfter}`);
    }
}

class ResultsListView {
    view(vnode) {
        const results = vnode.attrs.results;
        const keyword = vnode.attrs.keyword;
        const manifest = vnode.attrs.manifest;
        const resultsView = m("div.results", results.slice(0, DISPLAY_LIMIT).map(result => m(ResultView, { result, keyword, manifest })));
        if (results.length > DISPLAY_LIMIT) {
            return [
                resultsView,
                m("hr"),
                m("div.truncated", `Hit display limit of ${DISPLAY_LIMIT}. Further results truncated.`)
            ];
        } else {
            return resultsView;
        }
    }
}

class ResultView {
    view(vnode) {
        const result = vnode.attrs.result;
        const keyword = vnode.attrs.keyword;
        const manifest = vnode.attrs.manifest;
        return [
            m("div.result", [
                m("div.side.left", [result.left]),
                m("div.center", [keyword]),
                m("div.side.right", [result.right]),
            ]),
            m(SourceView, { result, manifest })
        ];
    }
}

class SourceView {
    view(vnode) {
        const manifest = vnode.attrs.manifest;
        const result = vnode.attrs.result;
        if (!!manifest) {
            const source = manifest[result.filename];
            if (source !== undefined) {
                return m("div.source", [
                    m.trust(" &ndash; "),
                    m("a", { href: source.url}, [m("cite", [source.title])]),
                    ` (${source.author})`
                ]);
            }
        }
        // fall back to raw filename if manifest failed to load
        return m("div.source", [result.filename]);
    }
}

m.mount(document.getElementById("mithril"), PageView);
