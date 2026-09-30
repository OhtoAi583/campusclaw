-- 迭代 2 补充：token 方案的可撤销刷新令牌。
-- 访问令牌是无状态 JWT（短有效期），刷新令牌哈希入库，可轮换、可撤销。
CREATE TABLE IF NOT EXISTS refresh_tokens (
  id         CHAR(64)        NOT NULL,
  user_id    BIGINT UNSIGNED NOT NULL,
  created_at DATETIME        NOT NULL,
  expires_at DATETIME        NOT NULL,
  PRIMARY KEY (id),
  KEY idx_refresh_tokens_user_id (user_id),
  KEY idx_refresh_tokens_expires_at (expires_at),
  CONSTRAINT fk_refresh_tokens_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
