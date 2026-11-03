CREATE TABLE `sessions` (
  `token` text PRIMARY KEY,
  `data` blob NOT NULL,
  `expiry` real NOT NULL
);

CREATE INDEX `sessions_expiry_idx` ON `sessions`(`expiry`);
