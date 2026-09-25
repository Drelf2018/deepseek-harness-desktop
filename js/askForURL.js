// The window runs this file and then a call to askForURL, with the address it is showing as the
// argument. The name below is the one main.go calls, so the two move together: renamed here, the
// call stops resolving, and the only sign is a console error nobody is watching.
function askForURL(current) {
    var u = prompt('设置访问地址', current);
    if (u === null) return;
    u = u.trim();
    if (u && u !== current) window._setURL(u);
}
