-- 演示种子数据：仅当 CK_SEED_DEMO=true 时由 user 服务执行，生产禁用。不是迁移（文件名不符合 NNNN_name.sql）。
-- 幂等，可重复执行。口令是公开的测试值，不是秘密：
--   demo  / demo1234   （id 10000，普通用户）
--   admin / admin1234  （id 10001，管理员）
-- 哈希为 bcrypt cost 10，迁移测试会断言它们能校验通过。
-- 使用 schema 全限定名，调用方无需设置 search_path。

INSERT INTO usr.users (id, username, password_hash, phone, email, role, status)
VALUES
    (10000, 'demo',  '$2a$10$jeemfzTyrwclV.LiLk3MiuNtVQuizGV.p/KfWAOhAB4IYra5mYSaG', '13800000000', 'demo@couponkill.local',  'user',  1),
    (10001, 'admin', '$2a$10$1H0XHeCxU3Cn40EnLP/FLe9M2.nTpHdrfOtxrz3Hc.WxPredqsquC', '13800000001', 'admin@couponkill.local', 'admin', 1)
ON CONFLICT (username) DO NOTHING;
