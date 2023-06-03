window.addEventListener("DOMContentLoaded", () => {
    const finalHop = document.querySelector("#final-hop");

    // We only want the link to work after it's cleaned. We re-enable at the bottom.
    const finalHopLink = document.querySelector("#final-hop a");
    finalHopLink.removeAttribute("href");

    const removedParamsSpan = document.querySelector("#removed-params");

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
                "_kx",
                "cid",
                "ck_subscriber_id",
                "cmpid",
                "ea.tracking.id",
                "fbclid",
                "gclid",
                "mailId",
                "msclkid",
                "mc_cid",
                "mc_eid",
            ];

            const isBadPart =
                badParts.includes(param) ||
                param.startsWith("cm_") ||
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
        const sanitizedURLNode = document.createTextNode(sanitizedURL);
        // Find the <span> element
        const spanElement = document.querySelector('.rawFinalUrl');
        // Find the <strong> element inside the <span>
        const strongElement = spanElement.querySelector('strong');
        // Find the text node inside the <strong> element
        const textNode = strongElement.firstChild;
        // Insert the sanitized URL after the text node
        spanElement.parentNode.insertBefore(sanitizedURLNode, spanElement.nextSibling);

        // Create a new anchor element to store the modified URL
        const anchorElement = document.createElement("a");
        const encodedGoodParamString = encodeURIComponent(goodParamString);
        const newHref = baseUrl + encodedGoodParamString;

        // Sanitize the newHref using DOMPurify
        const sanitizedHref = DOMPurify.sanitize(newHref);
        const fixedURL = sanitizedHref.replace(/%3F/g, '?');

        anchorElement.setAttribute("href", fixedURL);
        anchorElement.setAttribute("target", "_blank");
        anchorElement.textContent = `${baseUrl}${goodParamString}`;

        // Replace the existing finalHop content with the modified URL
        finalHop.innerHTML = ""; // Clear any existing content
        finalHop.appendChild(anchorElement);
    }
});
