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
