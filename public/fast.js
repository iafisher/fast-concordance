const DISPLAY_LIMIT = 10000;

const GENERIC_ERROR_MESSAGE = "Sorry, there was an error.";

async function getManifest() {
    return {
        "bram-stoker_dracula": {
            author: "Bram Stoker",
            title: "Dracula",
            url: "https://standardebooks.org/ebooks/bram-stoker/dracula"
        }
    };
}

async function search(keyword, resultsOut, statsOut) {
    const startTime = performance.now();
    const httpResult = await fetch(`/concordance/concord?w=${encodeURIComponent(keyword)}`);
    if (!httpResult.ok) {
        const data = await httpResult.json();
        throw { error: data.error };
    }

    // TODO: Can I close the connection when display limit is hit and cancel request on server?
    const reader = httpResult.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
        const { done, value } = await reader.read();
        if (done) {
            statsOut.millisToLastResult = performance.now() - startTime;
            if (statsOut.millisToFirstResult === null) {
                // in case we got 0 results
                statsOut.millisToFirstResult = statsOut.millisToLastResult;
            }
            // TODO: does this drop the last result? b/c buffer might not be empty
            m.redraw();
            break;
        }
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
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
                resultsOut.push(data);
                m.redraw();

                if (statsOut.millisToFirstResult === null) {
                    statsOut.millisToFirstResult = performance.now() - startTime;
                }
            }
        }
    }
}

class PageView {
    constructor() {
        this.keyword = "";
        this.results = [];
        this.stats = { millisToFirstResult: null, millisToLastResult: null };
        this.error = null;
        this.manifest = null;

        getManifest().then(manifest => { this.manifest = manifest });
    }

    view() {
        return m("main", [
            m(InputView, { onEnter: (keyword) => this.onEnter(keyword) }),
            m(StatsView, { stats: this.stats, resultsCount: this.results.length }),
            this.error !== null
                ? m(ErrorView, { error: this.error })
                : m(ResultsListView, { keyword: this.keyword, results: this.results, manifest: this.manifest })]);
    }

    onEnter(keyword) {
        this.keyword = keyword;
        this.results = [];
        this.stats.millisToFirstResult = null;
        this.stats.millisToLastResult = null;
        this.error = null;
        search(this.keyword, this.results, this.stats).catch((e) => {
            console.log(e);
            if (typeof e.error.message === "string") {
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

class InputView {
    constructor(vnode) {
        this.value = "";
        this.onEnter = vnode.attrs.onEnter;
    }

    view() {
        return m("input", { placeholder: "Type 4 letters or more...", onkeydown: (e) => this.onkeydown(e) });
    }

    onkeydown(e) {
        if (e.keyCode === 13) {
            const keyword = e.target.value;
            e.target.value = "";
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
        if (results.length > DISPLAY_LIMIT) {
            return [
                m("div.results", results.slice(0, DISPLAY_LIMIT).map(result => m(ResultView, { result, keyword }))),
                m("hr"),
                m("div.truncated", `Hit display limit of ${DISPLAY_LIMIT}. Further results truncated.`)
            ];
        } else {
            return m("div.results", results.map(result => m(ResultView, { result, keyword })));
        }
    }
}

class ResultView {
    view(vnode) {
        const result = vnode.attrs.result;
        const keyword = vnode.attrs.keyword;
        return [
            m("div.result", [
                m("div.side.left", [result.left]),
                m("div.center", [keyword]),
                m("div.side.right", [result.right]),
            ]),
            m(SourceView, { result })
        ];
    }
}

class SourceView {
    view(vnode) {
        const result = vnode.attrs.result;
        if (window.ebooksManifest !== null) {
            const source = window.ebooksManifest[result.filename];
            if (source !== undefined) {
                return m("div.source", [
                    m("a", { href: source.url}, [m("cite", [source.title])]),
                    ` (${source.author})`
                ]);
            }
        }
        return m("div.source", [result.filename]);
    }
}

m.mount(document.body, PageView);
getManifest().then(m => {
    window.ebooksManifest = m;
});
