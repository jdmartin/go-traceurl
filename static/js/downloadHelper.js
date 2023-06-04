window.addEventListener("DOMContentLoaded", async () => {
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
        console.log(rawFinalURL)
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
})