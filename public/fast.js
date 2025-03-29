const elementButton = document.getElementById("search-btn");
const elementInput = document.getElementById("search");
const elementResults = document.getElementById("results");

elementButton.addEventListener("click", () => {
    const keyword = elementInput.value.trim();
    console.log(keyword);
    // TODO: safe URL construction
    fetch(`http://localhost:8722/concord?w=${keyword}`)
    .then(res => res.json())
    .then(data => {
        elementInput.value = "";
        elementResults.innerHTML = "";
        for (let d of data) {
            for (let m of d.Matches) {
                const p = document.createElement("p");
                p.textContent = `${m.Left} ${d.Keyword} ${m.Right}`;
                elementResults.append(p);
            }
        }
    })
});