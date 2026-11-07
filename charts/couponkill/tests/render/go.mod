// charts/couponkill 渲染断言（helm template 输出的离线检查），不进 chart 包（见 .helmignore）
module couponkill/charts/tests/render

go 1.27.0

require gopkg.in/yaml.v3 v3.0.1
