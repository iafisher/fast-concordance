const elementButton = document.getElementById("search-btn");
const elementInput = document.getElementById("search");
const elementResults = document.getElementById("results");

elementButton.addEventListener("click", () => {
    const keyword = elementInput.value.trim();
    elementInput.value = "";
    elementResults.innerHTML = "";
    // TODO: safe URL construction
    fetch(`http://localhost:8722/concord?w=${keyword}`)
    .then(res => {
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        function read() {
            reader.read().then(({ done, value }) => {
                if (done) {
                    return;
                }
                buffer += decoder.decode(value, { stream: true });
                const lines = buffer.split("\n");
                buffer = lines.pop();
                lines.forEach(line => {
                    const trimmed = line.trim();
                    if (trimmed) {
                        let data;
                        try {
                            data = JSON.parse(trimmed);
                        } catch (e) {
                            console.error("error decoding JSON", e, trimmed);
                            return;
                        }
                        const p = document.createElement("p");
                        p.textContent = `${data.left} ${keyword} ${data.right}`;
                        elementResults.append(p);
                    }
                });
                read();
            });
        }

        read();
    });
});