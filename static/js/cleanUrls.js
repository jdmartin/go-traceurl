window.addEventListener("DOMContentLoaded", () => {
    const finalHop = document.querySelector("#final-hop");
    const removedParamsSpan = document.querySelector("#removed-params");
    const rawFinal = document.querySelector(".rawFinalUrl");

    if (finalHop) {
        // Extract the URL from the final hop
        const url = finalHop.querySelector("a").textContent;
        const lastSlashIndex = url.lastIndexOf("/");
        const baseUrl = url.substring(0, lastSlashIndex + 1);
        const removedParams = [];

        function extractParameters(url) {
            let goodParams = "";
            let additionalText = "";

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
                } else {
                    additionalText += segment;
                }
            }

            // Let's just make sure the first character in goodParams is a ?
            if (goodParams.slice(1).length > 0) {
                additionalText += "?" + goodParams.slice(1);
            }

            return additionalText;
        }

        function filterTheParams(param) {
            // List of known bad parts to discard
            const badParts = [
                "cid",
                "ck_subscriber_id",
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

        // Create a new text node to show the pre-sanitized URL
        const sanitizedURL = DOMPurify.sanitize(url);
        rawFinal.innerHTML = "";
        rawFinal.textContent = "☢️ Uncleaned Final URL: " + sanitizedURL;

        // Create a new anchor element to store the modified URL
        const anchorElement = document.createElement("a");
        const encodedGoodParamString = encodeURIComponent(goodParamString);
        const newHref = baseUrl + encodedGoodParamString;

        // Sanitize the newHref using DOMPurify
        const sanitizedHref = DOMPurify.sanitize(newHref);

        anchorElement.setAttribute("href", sanitizedHref);
        anchorElement.setAttribute("target", "_blank");
        anchorElement.textContent = `${baseUrl}${goodParamString}`;

        // Replace the existing finalHop content with the modified URL
        finalHop.innerHTML = ""; // Clear any existing content
        finalHop.appendChild(anchorElement);
    }
});
