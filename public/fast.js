// TODO: truncate at 10,000 results

async function search(keyword, resultsOut, statsOut) {
    const startTime = performance.now();
    // TODO: safe URL construction
    const httpResult = await fetch(`/concord?w=${keyword}`);
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
    }

    view() {
        return m("main", [
            m(InputView, { onEnter: (keyword) => this.onEnter(keyword) }),
            m(StatsView, { stats: this.stats, resultsCount: this.results.length }),
            m(ResultsView, { keyword: this.keyword, results: this.results })]);
    }

    onEnter(keyword) {
        this.keyword = keyword;
        this.results = [];
        this.stats.millisToFirstResult = null;
        this.stats.millisToLastResult = null;
        search(this.keyword, this.results, this.stats);
    }
}

class InputView {
    constructor(vnode) {
        this.value = "";
        this.onEnter = vnode.attrs.onEnter;
    }

    view() {
        return m("input", { onkeydown: (e) => this.onkeydown(e) });
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

class ResultsView {
    view(vnode) {
        const results = vnode.attrs.results;
        const keyword = vnode.attrs.keyword;
        return m("div.results",
            [
                m("div.side.left", results.map(result => m("div", result.left))),
                m("div.center", results.map(_ => m("div", keyword))),
                m("div.side.right", results.map(result => m("div", result.right))),
            ]);
    }
}

m.mount(document.body, PageView);