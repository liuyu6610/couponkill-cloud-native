package com.aliyun.seckill.common.cache;

import com.google.common.hash.BloomFilter;
import com.google.common.hash.Funnels;
import org.springframework.stereotype.Component;
import javax.annotation.PostConstruct;
import java.nio.charset.StandardCharsets;

        @Component
public class BloomFilterUtil {

          private BloomFilter<String> bloomFilter;

           @PostConstruct
 public void init() {
                bloomFilter = BloomFilter.create(Funnels.stringFunnel(StandardCharsets.UTF_8), 100000, 0.01);
           }

           public void put(String key) {
               bloomFilter.put(key);
           }

           public boolean mightContain(String key) {
              return bloomFilter.mightContain(key);
           }
}