package com.ken.agent.framework;

import com.ken.agent.framework.errorcode.BaseErrorCode;
import org.junit.jupiter.api.Test;

class FrameworkApplicationTests {

    @Test
    void baseErrorCodeExposesCodeAndMessage() {
        org.junit.jupiter.api.Assertions.assertEquals("B000001", BaseErrorCode.SERVICE_ERROR.code());
        org.junit.jupiter.api.Assertions.assertNotNull(BaseErrorCode.SERVICE_ERROR.message());
    }

}
