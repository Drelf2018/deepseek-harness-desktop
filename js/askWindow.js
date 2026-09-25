fetch(location.href, { credentials: 'same-origin' })
    .then((r) => window._probe(r.status))
    .catch(() => window._probe(0))
