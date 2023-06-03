window.addEventListener("DOMContentLoaded", () => {
    // Make sure we use one of the correct protocols in our links.
    function validateForm() {
        var urlInput = document.getElementById("urlInput");
        var url = urlInput.value;

        // Perform client-side validation checks
        if (!/^https?:\/\//i.test(url)) {
            alert("Invalid URL format. Please include 'http://' or 'https://' in the URL.");
            urlInput.focus();
            return false;
        }

        // Look, this is more about expectations than security. I get that.
        // Additional validation checks can be added here
        // URL is valid, allow form submission
        return true;
    }

});