-- Web Push subscriptions, one per installed app or browser.
CREATE TABLE `push_subscriptions` (
  `endpoint` text PRIMARY KEY,
  `user_id` bigint NOT NULL REFERENCES `users`(`id`) ON DELETE CASCADE,
  `p256dh` text NOT NULL,
  `auth` text NOT NULL,
  `created_at` bigint NOT NULL
);

CREATE INDEX `push_subscriptions_user_idx` ON `push_subscriptions`(`user_id`);

-- The VAPID key pair identifying this server to push services. Generated on
-- first start; changing it invalidates all subscriptions.
CREATE TABLE `vapid_keys` (
  `id` integer PRIMARY KEY CHECK (`id` = 1),
  `public_key` text NOT NULL,
  `private_key` text NOT NULL
);
