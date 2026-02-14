enum ApiErrorKind {
  network,
  timeout,
  cancelled,
  unauthorized,
  forbidden,
  invalidRequest,
  conflict,
  unavailable,
  server,
  invalidResponse,
  unknown,
}

class ApiException implements Exception {
  const ApiException({
    required this.message,
    required this.kind,
    this.code,
    this.statusCode,
    this.requestPath,
    this.method,
    this.cause,
  });

  final String message;
  final ApiErrorKind kind;
  final String? code;
  final int? statusCode;
  final String? requestPath;
  final String? method;
  final Object? cause;

  bool get isTransient {
    switch (kind) {
      case ApiErrorKind.network:
      case ApiErrorKind.timeout:
      case ApiErrorKind.unavailable:
      case ApiErrorKind.server:
        return true;
      default:
        return false;
    }
  }

  bool get isAuthFailure {
    if (statusCode == 401) {
      return true;
    }

    switch (code) {
      case 'invalid_credentials':
      case 'unauthorized':
      case 'session_not_active':
      case 'refresh_reuse_detected':
        return true;
      default:
        return false;
    }
  }

  @override
  String toString() {
    return 'ApiException(kind: $kind, statusCode: $statusCode, code: $code, method: $method, path: $requestPath, message: $message)';
  }
}
