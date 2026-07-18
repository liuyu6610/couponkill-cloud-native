package com.aliyun.seckill.couponkillconnectorservice.service;

import com.aliyun.seckill.common.connector.PlatformType;
import com.aliyun.seckill.couponkillconnectorservice.domain.CouponPriceMap;
import com.aliyun.seckill.couponkillconnectorservice.domain.CouponPriceMapCommand;
import com.aliyun.seckill.couponkillconnectorservice.mapper.CouponPriceMapMapper;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.util.StringUtils;

import java.util.List;

@Service
@RequiredArgsConstructor
public class PriceMapService {

    private final CouponPriceMapMapper priceMapMapper;

    public CouponPriceMap upsert(CouponPriceMapCommand cmd) {
        if (cmd.getCouponId() == null || cmd.getPlatform() == null
                || !StringUtils.hasText(cmd.getExternalSkuId())) {
            throw new IllegalArgumentException("couponId / platform / externalSkuId 不能为空");
        }
        String sku = cmd.getExternalSkuId().trim();
        String currency = StringUtils.hasText(cmd.getCurrency()) ? cmd.getCurrency().trim() : "CNY";
        boolean enabled = cmd.getEnabled() == null || Boolean.TRUE.equals(cmd.getEnabled());

        CouponPriceMap existing = priceMapMapper.selectByCouponPlatform(cmd.getCouponId(), cmd.getPlatform());
        if (existing == null) {
            CouponPriceMap row = new CouponPriceMap();
            row.setCouponId(cmd.getCouponId());
            row.setPlatform(cmd.getPlatform());
            row.setExternalSkuId(sku);
            row.setTitle(cmd.getTitle());
            row.setManualPrice(cmd.getManualPrice());
            row.setCurrency(currency);
            row.setEnabled(enabled);
            priceMapMapper.insert(row);
            return priceMapMapper.selectById(row.getId());
        }
        existing.setExternalSkuId(sku);
        existing.setTitle(cmd.getTitle());
        existing.setManualPrice(cmd.getManualPrice());
        existing.setCurrency(currency);
        existing.setEnabled(enabled);
        priceMapMapper.updateById(existing);
        return priceMapMapper.selectById(existing.getId());
    }

    public List<CouponPriceMap> listByCoupon(Long couponId) {
        if (couponId == null) {
            throw new IllegalArgumentException("couponId 不能为空");
        }
        return priceMapMapper.selectByCouponId(couponId);
    }

    public List<CouponPriceMap> listEnabledByCoupon(Long couponId) {
        return priceMapMapper.selectEnabledByCouponId(couponId);
    }

    public void delete(Long id) {
        if (id == null) {
            throw new IllegalArgumentException("id 不能为空");
        }
        priceMapMapper.deleteById(id);
    }
}
