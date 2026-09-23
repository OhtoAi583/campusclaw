-- 迭代 1 的数据模型：班级、用户、会话、材料与知识库正文。
-- class_id 在两处业务表上都是 NOT NULL 并建立索引，支撑按班查询与隔离验收。

CREATE TABLE IF NOT EXISTS classes (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name        VARCHAR(64)     NOT NULL,
  created_at  DATETIME        NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id),
  UNIQUE KEY uk_classes_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS users (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username      VARCHAR(64)     NOT NULL,
  password_hash VARCHAR(255)    NOT NULL,
  role          ENUM('teacher','student') NOT NULL,
  class_id      BIGINT UNSIGNED NOT NULL,
  created_at    DATETIME        NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id),
  UNIQUE KEY uk_users_username (username),
  KEY idx_users_class_id (class_id),
  CONSTRAINT fk_users_class FOREIGN KEY (class_id) REFERENCES classes (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sessions (
  id         CHAR(64)        NOT NULL,
  user_id    BIGINT UNSIGNED NOT NULL,
  created_at DATETIME        NOT NULL,
  expires_at DATETIME        NOT NULL,
  PRIMARY KEY (id),
  KEY idx_sessions_user_id (user_id),
  KEY idx_sessions_expires_at (expires_at),
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS materials (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  class_id      BIGINT UNSIGNED NOT NULL,
  title         VARCHAR(255)    NOT NULL,
  original_name VARCHAR(255)    NOT NULL,
  stored_name   VARCHAR(255)    NOT NULL,
  size_bytes    BIGINT UNSIGNED NOT NULL,
  uploaded_by   BIGINT UNSIGNED NOT NULL,
  created_at    DATETIME        NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id),
  KEY idx_materials_class_id (class_id),
  UNIQUE KEY uk_materials_stored_name (stored_name),
  CONSTRAINT fk_materials_class FOREIGN KEY (class_id) REFERENCES classes (id),
  CONSTRAINT fk_materials_uploader FOREIGN KEY (uploaded_by) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS knowledge_entries (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  material_id BIGINT UNSIGNED NOT NULL,
  class_id    BIGINT UNSIGNED NOT NULL,
  content     MEDIUMTEXT      NOT NULL,
  created_at  DATETIME        NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id),
  KEY idx_knowledge_class_id (class_id),
  KEY idx_knowledge_material_id (material_id),
  CONSTRAINT fk_knowledge_material FOREIGN KEY (material_id) REFERENCES materials (id) ON DELETE CASCADE,
  CONSTRAINT fk_knowledge_class FOREIGN KEY (class_id) REFERENCES classes (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
