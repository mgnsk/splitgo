// Register the service worker that shows an offline page when the network is
// down and displays push notifications.
if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {});
}

// Running as the installed app rather than in a browser tab.
const standalone =
  window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;

const pushSupported =
  "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;

// The installed app on iOS only gets PushManager when opened from the Home
// Screen, so this is false in a Safari tab.
async function currentSubscription() {
  if (!pushSupported) {
    return null;
  }
  const reg = await navigator.serviceWorker.ready;
  return reg.pushManager.getSubscription();
}

function postSubscription(path, sub) {
  return fetch(path, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(sub),
  }).then((resp) => {
    if (!resp.ok) {
      throw new Error(`${path}: ${resp.status}`);
    }
  });
}

function base64UrlToBytes(s) {
  const b64 = (s + "=".repeat((4 - (s.length % 4)) % 4)).replace(/-/g, "+").replace(/_/g, "/");
  return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
}

// subscribePush asks for permission and subscribes this device. On iOS it
// must be called directly in a tap handler, before any other await.
async function subscribePush(reg, vapidKey) {
  const sub = await reg.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: base64UrlToBytes(vapidKey),
  });
  try {
    await postSubscription("/push/subscribe", sub);
  } catch (err) {
    await sub.unsubscribe();
    throw err;
  }
}

// Notification settings on the account page.
async function initPushCard(card) {
  const status = card.querySelector("[data-push-status]");
  const button = card.querySelector("[data-push-toggle]");
  if (!pushSupported) {
    return; // The server-rendered text explains how to get support.
  }

  // Get the registration before any click so that subscribe() runs directly
  // in the tap handler: iOS only allows the permission prompt there.
  const reg = await navigator.serviceWorker.ready;

  const render = async () => {
    const sub = await reg.pushManager.getSubscription();
    if (Notification.permission === "denied") {
      status.textContent =
        "Notifications are blocked for splitgo. Allow them in your device or browser settings.";
      button.hidden = true;
    } else if (sub) {
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
      const sub = await reg.pushManager.getSubscription();
      if (sub) {
        await postSubscription("/push/unsubscribe", sub).catch(() => {});
        await sub.unsubscribe();
      } else {
        await subscribePush(reg, card.dataset.vapidKey);
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
  if (!pushSupported || !standalone || localStorage.getItem(promptDismissedKey)) {
    return;
  }
  if (Notification.permission !== "default") {
    return; // Already decided.
  }
  const reg = await navigator.serviceWorker.ready;
  if (await reg.pushManager.getSubscription()) {
    return;
  }
  const resp = await fetch("/push/key", { credentials: "same-origin" });
  if (!resp.ok) {
    return;
  }
  const vapidKey = (await resp.text()).trim();

  const close = () => {
    localStorage.setItem(promptDismissedKey, "1");
    prompt.hidden = true;
  };
  const enable = prompt.querySelector("[data-push-prompt-enable]");
  const status = prompt.querySelector("[data-push-prompt-status]");

  enable.addEventListener("click", async () => {
    enable.disabled = true;
    try {
      await subscribePush(reg, vapidKey);
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

// Keep the server's copy of this device's subscription current, e.g. after
// signing in as another user or when the browser rotated the subscription.
async function syncSubscription() {
  if (!document.querySelector(".topbar .nav") || sessionStorage.getItem("push-synced")) {
    return;
  }
  const sub = await currentSubscription();
  if (sub && Notification.permission === "granted") {
    await postSubscription("/push/subscribe", sub);
  }
  sessionStorage.setItem("push-synced", "1");
}

// Stop notifications for this device when signing out.
document.addEventListener("submit", async (event) => {
  const form = event.target;
  if (!pushSupported || !(form instanceof HTMLFormElement) || form.getAttribute("action") !== "/logout") {
    return;
  }
  // Cancel synchronously, the subscription lookup is async.
  event.preventDefault();
  try {
    // serviceWorker.ready never resolves without a service worker.
    const timeout = new Promise((resolve) => setTimeout(resolve, 3000, null));
    const sub = await Promise.race([currentSubscription(), timeout]);
    if (sub) {
      await postSubscription("/push/unsubscribe", sub);
      await sub.unsubscribe();
    }
  } catch (err) {
    console.error(err);
  }
  sessionStorage.removeItem("push-synced");
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
  syncSubscription().catch(console.error);
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
let navigating = false;

function refreshIfStale() {
  if (!standalone || formDirty || hiddenAt === 0) {
    return;
  }
  hiddenAt = 0;
  // Wait briefly: when the app was brought up by tapping a notification, the
  // service worker's navigate message follows and takes precedence.
  setTimeout(() => {
    if (!navigating) {
      window.location.reload();
    }
  }, 300);
}

// Tapping a notification while the app is open: go to its page. Assigning
// the current URL reloads it, so the view is fresh either way.
if ("serviceWorker" in navigator) {
  navigator.serviceWorker.addEventListener("message", (event) => {
    const data = event.data;
    if (!data || data.type !== "navigate" || typeof data.url !== "string" || !data.url.startsWith("/") || data.url.startsWith("//")) {
      return;
    }
    navigating = true;
    window.location.assign(data.url);
  });
  navigator.serviceWorker.startMessages();
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
  if (submitting || navigating || formDirty) {
    return;
  }
  if (document.visibilityState !== "visible") {
    livePending = true; // Refresh when the page is shown again.
    return;
  }
  // Coalesce bursts of changes into one refresh.
  clearTimeout(liveTimer);
  liveTimer = setTimeout(() => {
    if (submitting || navigating || formDirty) {
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

document.addEventListener("DOMContentLoaded", syncTimezone);
