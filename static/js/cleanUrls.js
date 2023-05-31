window.addEventListener("DOMContentLoaded", () => {
    const finalHop = document.querySelector("#final-hop");
    const removedParamsSpan = document.querySelector("#removed-params");

    // Extract the URL from the final hop
    const url = finalHop.querySelector("a").textContent;
    const lastSlashIndex = url.lastIndexOf("/");
    const baseUrl = url.substring(0, lastSlashIndex + 1);
    const removedParams = [];

    function extractParameters(url) {
        let goodParams = "";

        // Split the URL on the last slash
        const lastSlashIndex = url.lastIndexOf("/") + 1;
        const path = url.substring(lastSlashIndex);

        // Split the path into segments
        const segments = path.split(/[?&]/);

        // Iterate over the segments
        for (const segment of segments) {
            // Split the segment on the equals sign
            const [key, value] = segment.split("=");

            if (key && value) {
                if (filterTheParams(key)) {
                    goodParams += `&${key}=${value}`;
                }
            }
        }

        let fixedGoodParams = "";
        // Let's just make sure the first character in goodParams is a ?
        if (goodParams.slice(1).length > 0) {
            fixedGoodParams = "?" + goodParams.slice(1);
        }

        return fixedGoodParams;
    }

    function filterTheParams(param) {
        // List of known bad parts to discard
        const badParts = [
            "cid",
            "cmpid",
            "fbclid",
            "gclid",
            "msclkid",
            "mc_cid",
            "mc_eid",
        ];

        const isBadPart =
            badParts.includes(param) ||
            param.startsWith("pk_") ||
            param.startsWith("utm_");
        if (isBadPart) {
            removedParams.push(param);
            return false;
        }
        return true;
    }

    let goodParamString = extractParameters(url);

    // Sort the removedParams list
    removedParams.sort();

    // Update the removedParamsSpan if necessary
    if (removedParams.length > 0) {
        removedParamsSpan.textContent = removedParams.join(", ");
    }

    // Update the href and text content of the final hop with the modified URL
    finalHop.querySelector("a").href = `${baseUrl}${goodParamString}`;
    finalHop.querySelector("a").textContent = `${baseUrl}${goodParamString}`;
});
