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
