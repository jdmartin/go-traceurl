window.addEventListener("DOMContentLoaded", (event) => {
    (async function () {
        const tooltipContainers =
            document.querySelectorAll(".tooltip-container");
        const httpStatusCodes = await fetchHttpStatusCodes();

        async function fetchHttpStatusCodes() {
            try {
                const response = await fetch("data/http_status_codes.json");
                const json = await response.json();

                tooltipContainers.forEach((tooltipContainer) => {
                    const statusButton = tooltipContainer.querySelector(
                        ".status-code > #status-button"
                    );
                    const tooltip = tooltipContainer.querySelector(
                        ".status-code > .tooltip"
                    );

                    statusButton.addEventListener("mouseover", (event) => {
                        const statusCode = statusButton.textContent;
                        const message = json[statusCode]?.message;
                        if (message) {
                            // Update the tooltip text content
                            tooltip.textContent = `Status: ${message}`;

                            // Show the tooltip
                            tooltip.style.visibility = "visible";
                        }
                    });

                    statusButton.addEventListener("mouseout", (event) => {
                        // Hide the tooltip on mouseout
                        tooltip.style.visibility = "hidden";
                    });
                });

                return json;
            } catch (error) {
                console.error("Error fetching HTTP status codes:", error);
                return {};
            }
        }
    })();
});
