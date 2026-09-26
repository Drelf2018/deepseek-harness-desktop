// The three panels that wait on a person - an approval, a question, a plan under review - are
// only visible from inside the page, so the page is what says when one appears. The app
// installs this into every document it is about to create, which is why nothing here may
// assume the DOM is ready and why the guard below is needed: the same script runs again in
// each new document, and a page reload is a new document.
//
// Each panel carries a key of its own (data-*-key): that key is what tells a new request from
// the same one re-rendered, which is the difference between one notification and a stream.
(function () {
    if (window.__dshPromptWatcher) return;
    window.__dshPromptWatcher = true;

    var announced = {};

    function text(root, selector) {
        var el = root.querySelector(selector);
        return el ? el.textContent : "";
    }

    // The toast body is one short line. The frontend's own text is markdown, so the whitespace
    // in it is meaningful to nobody here.
    function summarize(value, fallback) {
        var line = (value || "").replace(/\s+/g, " ").trim();
        if (!line) line = fallback;
        return line.length > 120 ? line.slice(0, 120) + "…" : line;
    }

    // The headline is the first element inside the scroll box: that is where the panel puts
    // the reason it is waiting.
    function headline(root, selector) {
        var box = root.querySelector(selector);
        if (!box) return "";
        return (box.firstElementChild || box).textContent;
    }

    function describe(panel) {
        if (panel.hasAttribute("data-approval-key")) {
            return ["等待审批", summarize(headline(panel, "[data-approval-scroll]"), "需要你点击「允许一次」或「拒绝」")];
        }
        if (panel.hasAttribute("data-plan-review-key")) {
            var plan = text(panel, "[data-plan-review-scroll] h1, [data-plan-review-scroll] h2, [data-plan-review-scroll] h3");
            return ["需要评审计划", summarize(plan, "请选择 Approve 或 Keep planning")];
        }
        return ["需要回答问题", summarize(text(panel, "h2"), "DSH 需要你回答问题或做出选择")];
    }

    function scan() {
        var panels = document.querySelectorAll(
            "[data-approval-key], [data-question-key], [data-plan-review-key]"
        );
        var live = {};
        for (var i = 0; i < panels.length; i++) {
            var panel = panels[i];
            var key =
                panel.getAttribute("data-approval-key") ||
                panel.getAttribute("data-question-key") ||
                panel.getAttribute("data-plan-review-key");
            live[key] = true;
            if (announced[key]) continue;
            announced[key] = true;
            var described = describe(panel);
            try {
                if (typeof window._notify === "function") {
                    window._notify(described[0], described[1]);
                }
            } catch (e) {
                // The binding is the app's; a page that has replaced it is the page's business.
            }
        }
        // A key that is gone is forgotten, so the same panel appearing again is announced
        // again - which is what a second request looks like.
        announced = live;
    }

    function start() {
        new MutationObserver(scan).observe(document.documentElement, { childList: true, subtree: true });
        // The observer is what normally finds a panel. This is the net under it, for a panel
        // put in place without a mutation anyone watching the document root can see.
        setInterval(scan, 1500);
        scan();
    }

    if (document.documentElement) start();
    else window.addEventListener("DOMContentLoaded", start);
})();
