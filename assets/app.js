// Register the service worker that shows an offline page when the network is
// down and displays push notifications.
if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {});
}

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
        const created = await reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: base64UrlToBytes(card.dataset.vapidKey),
        });
        try {
          await postSubscription("/push/subscribe", created);
        } catch (err) {
          await created.unsubscribe();
          throw err;
        }
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
  const card = document.getElementById("push");
  if (card) {
    initPushCard(card).catch(console.error);
  }
  syncSubscription().catch(console.error);
});

// Reload the installed app when it comes back to the foreground so balances
// and expenses are never stale. iOS resumes the app showing the page as it
// was when it was left. Pages with unsaved form input are left alone.
const standalone =
  window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;

let formDirty = false;
document.addEventListener("input", (event) => {
  if (event.target instanceof Element && event.target.closest("main form")) {
    formDirty = true;
  }
});

let hiddenAt = 0;
function refreshIfStale() {
  if (!standalone || formDirty || hiddenAt === 0) {
    return;
  }
  hiddenAt = 0;
  window.location.reload();
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
