// const elementButton = document.getElementById("search-btn");
// const elementInput = document.getElementById("search");
// const elementResults = document.getElementById("results");


async function search(keyword, resultsOut) {
    const startTime = performance.now();
    let firstResultTime = null;
    // TODO: safe URL construction
    const httpResult = await fetch(`/concord?w=${keyword}`);
    const reader = httpResult.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
        const { done, value } = await reader.read();
        if (done) {
            // TODO: does this drop the last result?
            console.log("time to last result: %f ms", (performance.now() - startTime).toFixed(1));
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

                if (firstResultTime === null) {
                    firstResultTime = performance.now();
                    console.log("time to first result: %f ms", (firstResultTime - startTime).toFixed(1))
                }
            }
        }
    }
}

class PageView {
    constructor() {
        this.keyword = "";
        this.results = [];
    }

    view() {
        return m("main", [
            m(InputView, { onEnter: (keyword) => this.onEnter(keyword) }),
            m(ResultsView, { keyword: this.keyword, results: this.results })]);
    }

    onEnter(keyword) {
        this.keyword = keyword;
        search(this.keyword, this.results);
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