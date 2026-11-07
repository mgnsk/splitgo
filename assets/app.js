// The service worker, notification taps and the push API come from pwa.js,
// which is loaded before this script.
const { pwa, push } = window.webkit;

// Notification settings on the account page.
async function initPushCard(card) {
  const status = card.querySelector("[data-push-status]");
  const button = card.querySelector("[data-push-toggle]");

  // Get the device before any click so that subscribe() runs directly in
  // the tap handler: iOS only allows the permission prompt there.
  const device = await push.device();
  if (!device) {
    return; // The server-rendered text explains how to get support.
  }

  const render = async () => {
    if (Notification.permission === "denied") {
      status.textContent =
        "Notifications are blocked for splitgo. Allow them in your device or browser settings.";
      button.hidden = true;
    } else if (await device.subscribed()) {
      status.textContent = "Notifications are on for this device.";
      button.textContent = "Turn off";
      button.hidden = false;
    } else {
      status.textContent = "Notifications are off for this device.";
      button.textContent = "Turn on notifications";
      button.hidden = false;
    }
  };

  button.addEventListener("click", async () => {
    button.disabled = true;
    try {
      if (await device.subscribed()) {
        await device.unsubscribe();
      } else {
        await device.subscribe();
      }
    } catch (err) {
      console.error(err);
      if (Notification.permission !== "denied") {
        status.textContent = "Could not change notifications, please try again.";
        button.disabled = false;
        return;
      }
    }
    button.disabled = false;
    await render();
  });

  await render();
}

// Offer notifications once in the installed app, where first-time users
// would not otherwise find the switch on the account page.
const promptDismissedKey = "push-prompt-dismissed";

async function initPushPrompt(prompt) {
  if (!push.supported || !pwa.standalone || localStorage.getItem(promptDismissedKey)) {
    return;
  }
  if (Notification.permission !== "default") {
    return; // Already decided.
  }
  const device = await push.device();
  if (!device || (await device.subscribed())) {
    return;
  }

  const close = () => {
    localStorage.setItem(promptDismissedKey, "1");
    prompt.hidden = true;
  };
  const enable = prompt.querySelector("[data-push-prompt-enable]");
  const status = prompt.querySelector("[data-push-prompt-status]");

  enable.addEventListener("click", async () => {
    enable.disabled = true;
    try {
      await device.subscribe();
      close();
    } catch (err) {
      console.error(err);
      if (Notification.permission === "denied") {
        close();
        return;
      }
      status.textContent = "Could not turn on notifications, please try again.";
      enable.disabled = false;
    }
  });
  prompt.querySelector("[data-push-prompt-dismiss]").addEventListener("click", close);

  prompt.hidden = false;
}

// Stop notifications for this device when signing out.
document.addEventListener("submit", async (event) => {
  const form = event.target;
  if (!push.supported || !(form instanceof HTMLFormElement) || form.getAttribute("action") !== "/logout") {
    return;
  }
  // Cancel synchronously, the subscription lookup is async.
  event.preventDefault();
  await push.unsubscribe().catch(console.error);
  form.submit();
});

document.addEventListener("DOMContentLoaded", () => {
  const prompt = document.getElementById("push-prompt");
  if (prompt) {
    initPushPrompt(prompt).catch(console.error);
  }
  const card = document.getElementById("push");
  if (card) {
    initPushCard(card).catch(console.error);
  }
  // Signed in: keep the server's copy of this device's subscription current.
  if (document.querySelector(".topbar .nav")) {
    push.sync().catch(console.error);
  }
});

// Reload the installed app when it comes back to the foreground so balances
// and expenses are never stale. iOS resumes the app showing the page as it
// was when it was left. Pages with unsaved form input are left alone.
let formDirty = false;
document.addEventListener("input", (event) => {
  if (event.target instanceof Element && event.target.closest("main form")) {
    formDirty = true;
  }
});

let hiddenAt = 0;

function refreshIfStale() {
  if (!pwa.standalone || formDirty || hiddenAt === 0) {
    return;
  }
  hiddenAt = 0;
  // Wait briefly: when the app was brought up by tapping a notification, the
  // service worker's navigate message follows and takes precedence.
  setTimeout(() => {
    if (!pwa.navigating) {
      window.location.reload();
    }
  }, 300);
}

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") {
    hiddenAt = Date.now();
  } else {
    refreshIfStale();
  }
});

// Restored from the back/forward cache, e.g. after switching apps on iOS.
window.addEventListener("pageshow", (event) => {
  if (event.persisted) {
    hiddenAt = hiddenAt || Date.now();
    refreshIfStale();
  }
});

// Live updates: the server sends a refresh event to open pages when someone
// changes a group of the user, and the page re-fetches its content in place.
// Pages with unsaved input and pages being left are not touched.
let submitting = false;
document.addEventListener("submit", (event) => {
  if (!event.defaultPrevented) {
    submitting = true;
  }
});

let liveTimer = 0;
let livePending = false;

function liveRefresh() {
  if (submitting || pwa.navigating || formDirty) {
    return;
  }
  if (document.visibilityState !== "visible") {
    livePending = true; // Refresh when the page is shown again.
    return;
  }
  // Coalesce bursts of changes into one refresh.
  clearTimeout(liveTimer);
  liveTimer = setTimeout(() => {
    if (submitting || pwa.navigating || formDirty) {
      return;
    }
    const url = window.location.pathname + window.location.search;
    if (window.htmx) {
      window.htmx
        .ajax("GET", url, { target: "main", select: "main", swap: "outerHTML" })
        .catch(() => window.location.reload());
    } else {
      window.location.reload();
    }
  }, 300);
}

let liveSource = null;

function connectLive() {
  if (liveSource || !window.EventSource || !document.querySelector(".topbar .nav")) {
    return;
  }
  // EventSource reconnects by itself when the server ends the stream.
  liveSource = new EventSource("/events");
  liveSource.addEventListener("refresh", liveRefresh);
}

function disconnectLive() {
  if (liveSource) {
    liveSource.close();
    liveSource = null;
  }
}

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible" && livePending) {
    livePending = false;
    liveRefresh();
  }
});
window.addEventListener("pagehide", disconnectLive);
window.addEventListener("pageshow", (event) => {
  if (event.persisted) {
    // Back on a page that was left by submitting a form.
    submitting = false;
    connectLive();
  }
});
document.addEventListener("DOMContentLoaded", connectLive);

// Save the browser's time zone so that dates like "today" for new expenses
// follow the user's local day.
function syncTimezone() {
  const saved = document.documentElement.dataset.timezone;
  if (saved === undefined) {
    return; // Not signed in.
  }
  let tz = "";
  try {
    tz = Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch {
    return;
  }
  if (!tz || tz === saved) {
    return;
  }
  fetch("/timezone", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({ timezone: tz }),
  }).catch(() => {});
}

// Sign-in forms send the time zone so it is saved when signing in.
function fillTimezoneInputs() {
  let tz = "";
  try {
    tz = Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch {
    return;
  }
  for (const input of document.querySelectorAll('input[name="timezone"]')) {
    input.value = tz;
  }
}

document.addEventListener("DOMContentLoaded", () => {
  fillTimezoneInputs();
  syncTimezone();
});

// Suggestion dropdowns: an input with data-combo="<list id>" shows the
// list's options matching what has been typed. A native <datalist> isn't
// used because Firefox doesn't open it on click.
function comboParts(input) {
  const list = document.getElementById(input.dataset.combo);
  return list ? { list, items: [...list.querySelectorAll("li[data-value]")] } : null;
}

function comboClose(input) {
  const parts = comboParts(input);
  if (!parts) {
    return;
  }
  parts.list.hidden = true;
  input.setAttribute("aria-expanded", "false");
  for (const li of parts.items) {
    li.removeAttribute("aria-selected");
  }
}

function comboOpen(input) {
  const parts = comboParts(input);
  if (!parts) {
    return;
  }
  const query = input.value.trim().toLowerCase();
  let shown = 0;
  for (const li of parts.items) {
    const match = query === "" || li.textContent.toLowerCase().includes(query);
    // An exact match has nothing left to suggest.
    li.hidden = !match || li.dataset.value.toLowerCase() === query;
    li.removeAttribute("aria-selected");
    if (!li.hidden) {
      shown++;
    }
  }
  parts.list.hidden = shown === 0;
  input.setAttribute("aria-expanded", String(shown > 0));
}

function comboPick(input, li) {
  input.value = li.dataset.value;
  comboClose(input);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function comboInput(target) {
  return target instanceof HTMLInputElement && target.dataset.combo ? target : null;
}

document.addEventListener("focusin", (event) => {
  const input = comboInput(event.target);
  if (input) {
    comboOpen(input);
  }
});

document.addEventListener("focusout", (event) => {
  const input = comboInput(event.target);
  if (input) {
    comboClose(input);
  }
});

document.addEventListener("click", (event) => {
  const input = comboInput(event.target);
  if (input) {
    comboOpen(input); // Reopen after Escape or a pick.
  }
});

document.addEventListener("input", (event) => {
  const input = comboInput(event.target);
  if (input && document.activeElement === input) {
    comboOpen(input);
  }
});

// Pointerdown keeps focus in the input, so focusout doesn't close the list
// before the pick.
document.addEventListener("pointerdown", (event) => {
  const li = event.target instanceof Element ? event.target.closest(".combo-list li[data-value]") : null;
  if (!li) {
    return;
  }
  const input = document.querySelector(`input[data-combo="${li.parentElement.id}"]`);
  if (input) {
    event.preventDefault();
    comboPick(input, li);
  }
});

document.addEventListener("keydown", (event) => {
  const input = comboInput(event.target);
  if (!input) {
    return;
  }
  const parts = comboParts(input);
  if (!parts) {
    return;
  }
  const visible = parts.items.filter((li) => !li.hidden);
  const current = visible.findIndex((li) => li.getAttribute("aria-selected") === "true");

  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    if (parts.list.hidden) {
      comboOpen(input);
      return;
    }
    event.preventDefault();
    if (visible.length === 0) {
      return;
    }
    const step = event.key === "ArrowDown" ? 1 : -1;
    const next = (current + step + visible.length) % visible.length;
    visible[current]?.removeAttribute("aria-selected");
    visible[next].setAttribute("aria-selected", "true");
    visible[next].scrollIntoView({ block: "nearest" });
  } else if (event.key === "Enter" && !parts.list.hidden && current >= 0) {
    event.preventDefault();
    comboPick(input, visible[current]);
  } else if (event.key === "Escape" && !parts.list.hidden) {
    event.preventDefault();
    comboClose(input);
  }
});
