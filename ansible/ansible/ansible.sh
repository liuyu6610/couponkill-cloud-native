
export ALICLOUD_ACCESS_KEY="xxx"
export ALICLOUD_SECRET_KEY="yyy"
export ALICLOUD_REGION="cn-hangzhou"
# 可选：STS/AssumeRole
export ALICLOUD_ASSUME_ROLE_ARN="acs:ram::1234567890:role/ansible-executor"
export ALICLOUD_ASSUME_ROLE_SESSION_NAME="ansible-session"
export ALICLOUD_ASSUME_ROLE_SESSION_EXPIRATION="3600"
# Python 3.x + virtualenv（推荐）
python3 -m venv .venv && source .venv/bin/activate
pip install "ansible>=2.15" footmark

ansible-galaxy collection install -r collections/requirements.yml

# 列出所有识别到的主机与分组（含 tag_* / vpc_* / security_group_* 等）
ansible-inventory -i inventories/alicloud --list | head -n 60

# 只看 prod 环境 + web 角色交集
ansible -i inventories/alicloud "tag_env=prod:&tag_role=web" -m ping

ansible-playbook -i inventories/alicloud playbooks/bootstrap.yml \
  -l "tag_env=prod:&tag_role=web"
ansible -i inventories/alicloud "tag_env=prod:&tag_role=web" -m ping
ansible-playbook -i inventories/alicloud playbooks/bootstrap.yml -l "tag_env=prod"

# 滚动部署
ansible-playbook -i inventories/alicloud playbooks/deploy.yml \
  -l "tag_env=prod:&tag_app=yourapp" \
  -e deploy_strategy=rolling -e rolling_serial=25% -e app_version=2025.09.24-002
# 蓝绿部署
ansible-playbook -i inventories/alicloud playbooks/deploy.yml \
  -l "tag_env=prod:&tag_app=yourapp" \
  -e deploy_strategy=blue_green -e nginx_managed=true \
  -e bg_color_active=blue -e app_version=2025.09.24-004
# 金丝雀 5%，观察 10 分钟，再滚动 20%
ansible-playbook -i inventories/alicloud playbooks/deploy.yml \
  -l "tag_env=prod:&tag_app=yourapp" \
  -e deploy_strategy=canary -e canary_batch=5% -e promote_wait=600 \
  -e rolling_serial=20% -e app_version=2025.09.24-003
# 回滚
ansible-playbook -i inventories/alicloud playbooks/rollback.yml \
  -l "tag_env=prod:&tag_app=yourapp" -e rollback_steps=1


ansible-playbook -i inventories/alicloud playbooks/bootstrap.yml -l prometheus


# 1) 服务是否在跑
systemctl status ilogtail || systemctl status fluent-bit

# 2) 本机是否有 ARMS 探针（若启用）
grep -R "JAVA_TOOL_OPTIONS" /etc/systemd/system/{{ app_name }}.service.d/ || echo "container env check in deploy logs"

# 3) Prometheus 是否加载规则
curl -s http://prometheus-host:9090/-/rules | grep app.rules
