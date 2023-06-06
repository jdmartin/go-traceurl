// Parse and Clean the Final URL
window.addEventListener("DOMContentLoaded", () => {
    const finalHop = document.querySelector("#final-hop");
    const removedParamsSpan = document.querySelector("#removed-params");

    if (finalHop) {
        let finalHopLink;
        // We only want the link to work after it's cleaned. We re-enable at the bottom.
        finalHopLink = finalHop.querySelector("a");
        finalHopLink.removeAttribute("href");
        // Extract the URL from the final hop
        const url = finalHop.querySelector("a").textContent;
        const lastSlashIndex = url.lastIndexOf("/");
        const baseUrl = url.substring(0, lastSlashIndex + 1);
        const removedParams = [];
        const preAnchor = new URL(url);
        const anchor = preAnchor.hash;

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
                } else if (segment.startsWith("#")) {
                    continue
                } else {
                    additionalText += segment;
                }
            }

            // Add anchor as needed
            if (anchor) {
                additionalText += anchor;
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
                "EMLCID",
                "EMLDTL",
                "fbclid",
                "gclid",
                "linkID",
                "mailId",
                "msclkid",
                "mc_cid",
                "mcID",
                "mc_eid",
                "mgparam",
                "rfrr",
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
        const fixedURL = sanitizedHref.replace(/%3F/g, '?').replace(/%32/g, '#');


        anchorElement.setAttribute("href", fixedURL);
        anchorElement.setAttribute("target", "_blank");
        anchorElement.textContent = `${baseUrl}${goodParamString}`;

        // Replace the existing finalHop content with the modified URL
        finalHop.innerHTML = ""; // Clear any existing content
        finalHop.appendChild(anchorElement);
    }
});

// Generate the tooltips
window.addEventListener("DOMContentLoaded", async () => {
    const tooltipContainers = document.querySelectorAll(".tooltip-container");
    const httpStatusCodes = await fetchHttpStatusCodes();

    async function fetchHttpStatusCodes() {
        try {
            const response = await fetch("static/data/http_status_codes.json");
            const json = await response.json();
            return json;
        } catch (error) {
            console.error("Error fetching HTTP status codes:", error);
            return {};
        }
    }

    tooltipContainers.forEach((tooltipContainer) => {
        const statusButton = tooltipContainer.querySelector("#status-button");
        const infoIcon = tooltipContainer.querySelector(".info-icon");
        const tooltip = tooltipContainer.querySelector(".tooltip");

        infoIcon.addEventListener("mouseover", (event) => {
            const statusCode = statusButton.textContent;
            const message = httpStatusCodes[statusCode]?.message;
            if (message) {
                // Update the tooltip text content
                tooltip.textContent = `Status: ${message}`;

                // Show the tooltip
                tooltip.style.visibility = "visible";
            }
        });

        infoIcon.addEventListener("mouseout", (event) => {
            // Hide the tooltip on mouseout
            tooltip.style.visibility = "hidden";
        });
    });
});

// Create a toggle button for the meta info (raw final url, params removed, secret)
document.addEventListener('DOMContentLoaded', function () {
    // Get the button and the rows with class "result-info"
    var toggleButton = document.getElementById('toggleButton');
    var rows = document.getElementsByClassName('result-info');

    // Set the initial state of the rows
    for (var i = 0; i < rows.length; i++) {
        rows[i].style.display = 'none';
    }

    // Add event listener to the button
    toggleButton.addEventListener('click', function () {
        // Toggle the display property of the rows
        for (var i = 0; i < rows.length; i++) {
            if (rows[i].style.display === 'none') {
                rows[i].style.display = 'table-row';
            } else {
                rows[i].style.display = 'none';
            }
        }
    });
});

// Provide download to JSON
window.addEventListener("DOMContentLoaded", async () => {
    if (document.getElementById("downloadButton")) {
        document.getElementById("downloadButton").addEventListener("click", function () {
            // Get all table rows except those with the "result-info" class
            var rows = Array.from(document.querySelectorAll("#resultTable tbody tr:not(.result-info)"));

            // Prepare an array to store the hops
            var hops = [];

            // Loop through each row and extract the relevant values
            rows.forEach(function (row) {
                var cells = row.getElementsByTagName("td");
                var status = cells[1].textContent.trim();
                var cleanStatus = status.replace(/\D/g, '');

                var hop = {
                    "Hop": cells[0].textContent.trim(),
                    "Status": cleanStatus,
                    "URL": cells[2].textContent.trim()
                };
                hops.push(hop);
            });

            // Extract result-info rows data
            var rawFinalURL = document.querySelector(".rawFinalUrl").nextSibling.textContent.trim();
            var removedParams = document.getElementById("removed-params").textContent;

            // Convert the removedParams string to an array
            var removedParamsArray = removedParams.split(',').map(function (param) {
                return param.trim();
            });

            // Prepare the Results object
            var meta = {
                "rawFinalURL": rawFinalURL || null,
                "removedParams": removedParamsArray || null,
            };

            // Construct the final JSON structure
            var jsonData = {
                "Results": hops,
                "Meta": meta
            };

            // Convert the data to JSON
            var jsonString = JSON.stringify(jsonData, null, 2);

            // Create a download link and trigger the download
            var element = document.createElement("a");
            element.setAttribute("href", "data:text/plain;charset=utf-8," + encodeURIComponent(jsonString));
            element.setAttribute("download", "gotrace_data.json");
            element.style.display = "none";
            document.body.appendChild(element);
            element.click();
            document.body.removeChild(element);
        });
    } else {
        const controlsContainers = document.getElementsByClassName("controls-container");

        // Iterate over each controls-container element
        for (let i = 0; i < controlsContainers.length; i++) {
            const controlsContainer = controlsContainers[i];

            // Set the right property to 11%
            controlsContainer.style.right = "7.5%";
            controlsContainer.style.display = "inline";
        }
    }
})