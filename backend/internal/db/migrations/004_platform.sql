-- 迭代 3：AI 应用平台。应用 = 提示词模板 + 知识库绑定 + 技能 + 工具。
CREATE TABLE IF NOT EXISTS prompt_templates (
  id             VARCHAR(64)  NOT NULL,
  name           VARCHAR(128) NOT NULL,
  description    VARCHAR(255) NOT NULL DEFAULT '',
  system_template MEDIUMTEXT  NOT NULL,
  user_template   MEDIUMTEXT  NOT NULL,
  variables      VARCHAR(512) NOT NULL DEFAULT '',
  created_at     DATETIME     NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ai_apps (
  id                  VARCHAR(64)  NOT NULL,
  name                VARCHAR(128) NOT NULL,
  description         VARCHAR(255) NOT NULL DEFAULT '',
  icon                VARCHAR(16)  NOT NULL DEFAULT '✦',
  template_id         VARCHAR(64)  NOT NULL,
  model               VARCHAR(64)  NOT NULL,
  temperature         DOUBLE       NOT NULL DEFAULT 0.2,
  max_tokens          INT          NOT NULL DEFAULT 800,
  kb_enabled          TINYINT(1)   NOT NULL DEFAULT 1,
  kb_top_k            INT          NOT NULL DEFAULT 4,
  sort_order          INT          NOT NULL DEFAULT 0,
  enabled             TINYINT(1)   NOT NULL DEFAULT 1,
  created_at          DATETIME     NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id),
  CONSTRAINT fk_ai_apps_template FOREIGN KEY (template_id) REFERENCES prompt_templates (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS skills (
  id              VARCHAR(64)  NOT NULL,
  name            VARCHAR(128) NOT NULL,
  description     VARCHAR(255) NOT NULL DEFAULT '',
  body            MEDIUMTEXT   NOT NULL,
  builtin         TINYINT(1)   NOT NULL DEFAULT 0,
  enabled         TINYINT(1)   NOT NULL DEFAULT 1,
  source_filename VARCHAR(255) NOT NULL DEFAULT '',
  size_chars      INT          NOT NULL DEFAULT 0,
  created_at      DATETIME     NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS app_skills (
  app_id   VARCHAR(64) NOT NULL,
  skill_id VARCHAR(64) NOT NULL,
  PRIMARY KEY (app_id, skill_id),
  CONSTRAINT fk_app_skills_app FOREIGN KEY (app_id) REFERENCES ai_apps (id) ON DELETE CASCADE,
  CONSTRAINT fk_app_skills_skill FOREIGN KEY (skill_id) REFERENCES skills (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS mcp_servers (
  id          VARCHAR(64)  NOT NULL,
  name        VARCHAR(128) NOT NULL,
  command     VARCHAR(255) NOT NULL,
  args        VARCHAR(512) NOT NULL DEFAULT '',
  builtin     TINYINT(1)   NOT NULL DEFAULT 0,
  enabled     TINYINT(1)   NOT NULL DEFAULT 1,
  created_at  DATETIME     NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS app_mcp_servers (
  app_id    VARCHAR(64) NOT NULL,
  server_id VARCHAR(64) NOT NULL,
  PRIMARY KEY (app_id, server_id),
  CONSTRAINT fk_app_mcp_app FOREIGN KEY (app_id) REFERENCES ai_apps (id) ON DELETE CASCADE,
  CONSTRAINT fk_app_mcp_server FOREIGN KEY (server_id) REFERENCES mcp_servers (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 每次调用 LLM 的完整请求 / 响应，用于验收与排障（报告里的证据来源）。
CREATE TABLE IF NOT EXISTS llm_calls (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  app_id            VARCHAR(64)  NOT NULL,
  user_id           BIGINT UNSIGNED NOT NULL,
  class_id          BIGINT UNSIGNED NOT NULL,
  conversation_id   VARCHAR(64)  NOT NULL DEFAULT '',
  model             VARCHAR(64)  NOT NULL,
  request_json      MEDIUMTEXT   NOT NULL,
  response_json     MEDIUMTEXT   NOT NULL,
  status            VARCHAR(32)  NOT NULL DEFAULT '',
  latency_ms        INT          NOT NULL DEFAULT 0,
  prompt_tokens     INT          NOT NULL DEFAULT 0,
  completion_tokens INT          NOT NULL DEFAULT 0,
  tool_rounds       INT          NOT NULL DEFAULT 0,
  round_index       INT          NOT NULL DEFAULT 0,
  kb_chunks         INT          NOT NULL DEFAULT 0,
  kb_chars          INT          NOT NULL DEFAULT 0,
  skill_chars       INT          NOT NULL DEFAULT 0,
  tools_json        MEDIUMTEXT   NULL,
  created_at        DATETIME     NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id),
  KEY idx_llm_calls_app (app_id, created_at),
  KEY idx_llm_calls_conversation (conversation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
