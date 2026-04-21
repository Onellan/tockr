document.addEventListener("click", async function (event) {
  const el = event.target.closest("[hx-post],[hx-get]");
  if (!el) return;
  event.preventDefault();
  const target = document.querySelector(el.getAttribute("hx-target") || "body");
  const method = el.hasAttribute("hx-post") ? "POST" : "GET";
  const url = el.getAttribute(method === "POST" ? "hx-post" : "hx-get");
  const response = await fetch(url, { method, headers: { "HX-Request": "true" } });
  if (target) target.innerHTML = await response.text();
});

