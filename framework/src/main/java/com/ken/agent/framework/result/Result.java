package com.ken.agent.framework.result;

import com.ken.agent.framework.errorcode.BaseErrorCode;
import com.ken.agent.framework.exception.AbstractException;
import lombok.Data;
import lombok.experimental.Accessors;

import java.io.Serial;
import java.io.Serializable;
import java.util.Optional;

/**
 * 统一接口响应结果。
 *
 * @param <T> 响应数据类型
 */
@Data
@Accessors(chain = true)
public class Result<T> implements Serializable {

    @Serial
    private static final long serialVersionUID = 1L;

    /**
     * 成功响应码。
     */
    public static final String SUCCESS_CODE = "0";

    /**
     * 响应码。
     */
    private String code;

    /**
     * 响应消息。
     */
    private String message;

    /**
     * 响应数据。
     */
    private T data;

    /**
     * 构造成功响应。
     */
    public static Result<Void> success() {
        return new Result<Void>()
                .setCode(SUCCESS_CODE);
    }

    /**
     * 构造带返回数据的成功响应。
     */
    public static <T> Result<T> success(T data) {
        return new Result<T>()
                .setCode(SUCCESS_CODE)
                .setData(data);
    }

    /**
     * 构建服务端失败响应。
     */
    public static Result<Void> failure() {
        return new Result<Void>()
                .setCode(BaseErrorCode.SERVICE_ERROR.code())
                .setMessage(BaseErrorCode.SERVICE_ERROR.message());
    }

    /**
     * 通过 {@link AbstractException} 构建失败响应。
     */
    static Result<Void> failure(AbstractException abstractException) {
        String errorCode = Optional.ofNullable(abstractException.getErrorCode())
                .orElse(BaseErrorCode.SERVICE_ERROR.code());
        String errorMessage = Optional.ofNullable(abstractException.getErrorMessage())
                .orElse(BaseErrorCode.SERVICE_ERROR.message());
        return new Result<Void>()
                .setCode(errorCode)
                .setMessage(errorMessage);
    }

    /**
     * 通过 errorCode、errorMessage 构建失败响应。
     */
    static Result<Void> failure(String errorCode, String errorMessage) {
        return new Result<Void>()
                .setCode(errorCode)
                .setMessage(errorMessage);
    }
}
