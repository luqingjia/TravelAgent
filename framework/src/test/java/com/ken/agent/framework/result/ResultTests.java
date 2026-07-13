package com.ken.agent.framework.result;

import com.ken.agent.framework.errorcode.BaseErrorCode;
import com.ken.agent.framework.exception.ServiceException;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

class ResultTests {

    @Test
    void successBuildsEmptySuccessResponse() {
        Result<Void> result = Result.success();

        assertEquals(Result.SUCCESS_CODE, result.getCode());
        assertNull(result.getMessage());
        assertNull(result.getData());
    }

    @Test
    void successBuildsDataSuccessResponse() {
        Result<String> result = Result.success("ok");

        assertEquals(Result.SUCCESS_CODE, result.getCode());
        assertEquals("ok", result.getData());
    }

    @Test
    void failureBuildsDefaultServiceErrorResponse() {
        Result<Void> result = Result.failure();

        assertEquals(BaseErrorCode.SERVICE_ERROR.code(), result.getCode());
        assertEquals(BaseErrorCode.SERVICE_ERROR.message(), result.getMessage());
    }

    @Test
    void failureBuildsResponseFromAbstractException() {
        Result<Void> result = Result.failure(new ServiceException("upload failed"));

        assertEquals(BaseErrorCode.SERVICE_ERROR.code(), result.getCode());
        assertEquals("upload failed", result.getMessage());
    }

    @Test
    void failureBuildsResponseFromCodeAndMessage() {
        Result<Void> result = Result.failure("A000001", "bad request");

        assertEquals("A000001", result.getCode());
        assertEquals("bad request", result.getMessage());
    }
}
