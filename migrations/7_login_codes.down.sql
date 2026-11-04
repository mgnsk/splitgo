DROP TABLE `login_failures`;
ALTER TABLE `login_tokens` DROP COLUMN `attempts`;
ALTER TABLE `login_tokens` DROP COLUMN `code_hash`;
