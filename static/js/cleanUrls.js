window.addEventListener('DOMContentLoaded', (event) => {
    const finalHop = document.querySelector('#final-hop');
    const removedParamsSpan = document.querySelector('#removed-params');

    // Extract the URL from the final hop and remove UTM parameters
    const url = finalHop.querySelector('a').href;
    const urlWithoutUTM = removeTrackingParameters(url);

    // Update the href and text of the final hop with the modified URL
    finalHop.querySelector('a').href = urlWithoutUTM;
    finalHop.querySelector('a').textContent = urlWithoutUTM;

    function removeTrackingParameters(url) {
        const u = new URL(url);
        const params = u.searchParams;
        const trackingParams = [];
        const removedParams = [];
    
        params.forEach((value, key) => {
            if (
                key.startsWith('utm_') ||
                key === 'cmpid' ||
                key === 'cid' ||
                key === 'fbclid' ||
                key === 'gclid' ||
                key === 'msclkid' ||
                key === 'mc_cid' ||
                key === 'mc_eid' ||
                key.startsWith('pk_')
            ) {
                trackingParams.push(key);
            }
        });
    
        trackingParams.forEach((param) => {
            params.delete(param);
            removedParams.push(param);
        });
    
        if (params.toString() === trackingParams.join('&')) {
            // If the query string only contained tracking parameters, remove the entire query string
            u.search = '';
        }

        // Sort, then Populate the span with the comma-separated list of removed parameters
        if (removedParams.length > 0) {
            removedParams.sort();
            removedParamsSpan.textContent = removedParams.join(', ');
        }
    
        return u.toString();
    }
});