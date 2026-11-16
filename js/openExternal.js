// Links that leave this app, from both ends the page has:
//
//   - the ones it handles itself (its click handlers call window.open for an external address), and
//   - the ones it does not - a bare anchor, a linkified address, a markdown link from a component
//     that does not delegate - which go straight to the engine and come back as a chromeless popup
//     inside this app instead of the browser the user browses in.
//
// So a click is taken at the capture phase and window.open is wrapped as well. A click the page also
// answers with window.open would then be handed over twice, so the address the click took is
// remembered for one shot: the page's own call for that same address is the same click, and only
// suppresses the popup. The memory is one address deep and is cleared by the page's next call or
// the next click - a causal token, not a time window.
//
// Only what is outside this page is taken: an http(s) address on another origin, or a mailto
// address. about:blank popups (the account sign-in flow builds one and navigates it afterwards),
// blob: URLs (the document viewer), same-origin windows and the terminal's empty window.open are
// left exactly as they were, because they are the page talking to itself.
(function () {
    if (window.__dshExternalLinks) return;
    window.__dshExternalLinks = true;

    function outside(raw) {
        if (!raw) return false;
        var u;
        try {
            u = new URL(String(raw), location.href);
        } catch (e) {
            return false;
        }
        if (u.protocol === "mailto:") return true;
        if (u.protocol !== "http:" && u.protocol !== "https:") return false;
        return u.origin !== location.origin;
    }

    function normalized(raw) {
        try {
            return new URL(String(raw), location.href).href;
        } catch (e) {
            return String(raw);
        }
    }

    function hand(url) {
        try {
            if (typeof window._openExternal === "function") window._openExternal(url);
        } catch (e) {
            // The binding is the app's; a page that has replaced it is the page's business.
        }
    }

    var takenByClick = "";

    document.addEventListener("click", function (event) {
        if (event.defaultPrevented) return;
        var node = event.target;
        var a = node && node.closest ? node.closest("a[href]") : null;
        if (!a || a.hasAttribute("download")) return;
        if (!outside(a.href)) return;
        event.preventDefault();
        takenByClick = normalized(a.href);
        hand(a.href);
    }, true);

    var open = window.open;
    window.open = function (url) {
        if (outside(url)) {
            var same = takenByClick !== "" && takenByClick === normalized(url);
            takenByClick = "";
            if (!same) hand(String(url));
            return null;
        }
        return open.apply(window, arguments);
    };
})();
