package com.aliyun.seckill.couponkillconnectorservice.connector.jd;

import com.aliyun.seckill.common.connector.ConnectorHealth;
import com.aliyun.seckill.common.connector.EcommerceConnector;
import com.aliyun.seckill.common.connector.PlatformProductSnapshot;
import com.aliyun.seckill.common.connector.PlatformStockSnapshot;
import com.aliyun.seckill.common.connector.PlatformType;
import com.aliyun.seckill.couponkillconnectorservice.config.JdConnectorProperties;
import com.fasterxml.jackson.databind.JsonNode;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.math.BigDecimal;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * 京东开放平台 Connector。无完整密钥时 health=DISABLED，启动不失败。
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class JdEcommerceConnector implements EcommerceConnector {

    private final JdJosClient josClient;
    private final JdConnectorProperties props;

    @Override
    public PlatformType platform() {
        return PlatformType.JD;
    }

    @Override
    public PlatformProductSnapshot getProduct(String externalSkuId) {
        ensureReady();
        Map<String, String> biz = new LinkedHashMap<>();
        biz.put("skuId", externalSkuId);
        biz.put("field", "skuId,skuName,jdPrice,status,stockNum");
        JsonNode root = josClient.invoke("jingdong.sku.read.searchSkuList", biz);
        assertNoError(root, "searchSkuList");

        JsonNode sku = findFirstSku(root);
        if (sku == null || sku.isMissingNode()) {
            throw new IllegalStateException("京东未返回 SKU: " + externalSkuId);
        }
        BigDecimal price = null;
        if (sku.hasNonNull("jdPrice")) {
            price = new BigDecimal(sku.get("jdPrice").asText());
        }
        boolean onSale = true;
        if (sku.has("status")) {
            onSale = sku.get("status").asInt(1) == 1;
        }
        return PlatformProductSnapshot.builder()
                .platform(PlatformType.JD)
                .externalSkuId(externalSkuId)
                .title(textOr(sku, "skuName", "JD-" + externalSkuId))
                .price(price)
                .onSale(onSale)
                .rawStatus(sku.path("status").asText(null))
                .build();
    }

    @Override
    public PlatformStockSnapshot getStock(String externalSkuId) {
        ensureReady();
        // 优先商家库存只读；失败则降级到 searchSkuList 的 stockNum 字段
        try {
            Map<String, String> biz = new LinkedHashMap<>();
            biz.put("skuIds", externalSkuId);
            JsonNode root = josClient.invoke("jingdong.stock.read.searchSkuStock", biz);
            assertNoError(root, "searchSkuStock");
            JsonNode stockNode = findStockNode(root, externalSkuId);
            if (stockNode != null) {
                Long qty = stockNode.has("stockNum") ? stockNode.get("stockNum").asLong()
                        : (stockNode.has("remainNum") ? stockNode.get("remainNum").asLong() : null);
                boolean available = qty == null || qty > 0 || qty == -1L;
                return PlatformStockSnapshot.builder()
                        .platform(PlatformType.JD)
                        .externalSkuId(externalSkuId)
                        .stockQty(qty != null && qty >= 0 ? qty : null)
                        .stockAvailable(available)
                        .stockStateDesc(stockNode.path("stockStateDesc").asText("JD_STOCK"))
                        .area(props.getDefaultArea())
                        .build();
            }
            log.warn("searchSkuStock 无精确节点，降级 searchSkuList: skuId={}", externalSkuId);
        } catch (Exception e) {
            log.warn("searchSkuStock 失败，降级 searchSkuList: skuId={}, err={}", externalSkuId, e.getMessage());
        }

        PlatformProductSnapshot product = getProduct(externalSkuId);
        // getProduct 已调用 searchSkuList；再拉一次拿 stockNum
        Map<String, String> biz = new LinkedHashMap<>();
        biz.put("skuId", externalSkuId);
        biz.put("field", "skuId,stockNum,status");
        JsonNode root = josClient.invoke("jingdong.sku.read.searchSkuList", biz);
        JsonNode sku = findFirstSku(root);
        Long qty = sku != null && sku.has("stockNum") ? sku.get("stockNum").asLong() : null;
        return PlatformStockSnapshot.builder()
                .platform(PlatformType.JD)
                .externalSkuId(externalSkuId)
                .stockQty(qty)
                .stockAvailable(Boolean.TRUE.equals(product.getOnSale()) && (qty == null || qty > 0))
                .stockStateDesc("JD_FALLBACK_SKU_LIST")
                .area(props.getDefaultArea())
                .build();
    }

    @Override
    public ConnectorHealth health() {
        if (!props.isEnabled()) {
            return ConnectorHealth.builder()
                    .platform(PlatformType.JD)
                    .status("DISABLED")
                    .message("connector.jd.enabled=false")
                    .build();
        }
        if (!props.credentialsPresent()) {
            return ConnectorHealth.builder()
                    .platform(PlatformType.JD)
                    .status("DISABLED")
                    .message("缺少 JD_APP_KEY / JD_APP_SECRET / JD_ACCESS_TOKEN")
                    .build();
        }
        // 有密钥但未做真实 ping，避免前端误判「已联通」
        return ConnectorHealth.builder()
                .platform(PlatformType.JD)
                .status("CONFIGURED")
                .message("credentials configured（未探活，请用 /probe 验证）")
                .build();
    }

    private void ensureReady() {
        if (!props.credentialsPresent()) {
            throw new IllegalStateException("京东 Connector 未就绪：" + health().getMessage());
        }
    }

    private static void assertNoError(JsonNode root, String api) {
        if (root == null) {
            throw new IllegalStateException("京东空响应: " + api);
        }
        JsonNode err = root.path("error_response");
        if (!err.isMissingNode() && !err.isNull()) {
            throw new IllegalStateException("京东接口错误(" + api + "): " + err);
        }
    }

    private static JsonNode findFirstSku(JsonNode root) {
        // 兼容多种响应包装
        JsonNode[] candidates = new JsonNode[]{
                root.path("jingdong_sku_read_searchSkuList_responce").path("page").path("data"),
                root.path("jingdong_sku_read_searchSkuList_response").path("page").path("data"),
                root.path("page").path("data"),
                root.path("data")
        };
        for (JsonNode c : candidates) {
            if (c != null && c.isArray() && !c.isEmpty()) {
                return c.get(0);
            }
        }
        return null;
    }

    private static JsonNode findStockNode(JsonNode root, String skuId) {
        JsonNode[] candidates = new JsonNode[]{
                root.path("jingdong_stock_read_searchSkuStock_responce").path("stockList"),
                root.path("jingdong_stock_read_searchSkuStock_response").path("stockList"),
                root.path("stockList"),
                root.path("result")
        };
        for (JsonNode c : candidates) {
            if (c != null && c.isArray()) {
                for (JsonNode n : c) {
                    if (skuId.equals(n.path("skuId").asText()) || skuId.equals(n.path("sku_id").asText())) {
                        return n;
                    }
                }
                if (!c.isEmpty()) {
                    return c.get(0);
                }
            }
        }
        return null;
    }

    private static String textOr(JsonNode n, String field, String def) {
        return n.hasNonNull(field) ? n.get(field).asText() : def;
    }
}
