class AppConfig {
  const AppConfig({
    required this.apiBaseUrl,
    required this.inviteOnly,
    required this.webCookieMode,
    required this.csrfCookieName,
    required this.csrfHeaderName,
  });

  final String apiBaseUrl;
  final bool inviteOnly;
  final bool webCookieMode;
  final String csrfCookieName;
  final String csrfHeaderName;

  AppConfig copyWith({
    String? apiBaseUrl,
    bool? inviteOnly,
    bool? webCookieMode,
    String? csrfCookieName,
    String? csrfHeaderName,
  }) {
    return AppConfig(
      apiBaseUrl: apiBaseUrl ?? this.apiBaseUrl,
      inviteOnly: inviteOnly ?? this.inviteOnly,
      webCookieMode: webCookieMode ?? this.webCookieMode,
      csrfCookieName: csrfCookieName ?? this.csrfCookieName,
      csrfHeaderName: csrfHeaderName ?? this.csrfHeaderName,
    );
  }

  static const defaultApiBaseUrl = String.fromEnvironment(
    'ARC_API_BASE_URL',
    defaultValue: 'http://127.0.0.1:8080',
  );

  factory AppConfig.fromEnvironment() {
    return const AppConfig(
      apiBaseUrl: defaultApiBaseUrl,
      inviteOnly: bool.fromEnvironment(
        'ARC_AUTH_INVITE_ONLY',
        defaultValue: true,
      ),
      webCookieMode: bool.fromEnvironment(
        'ARC_AUTH_WEB_COOKIE_MODE',
        defaultValue: true,
      ),
      csrfCookieName: String.fromEnvironment(
        'ARC_AUTH_CSRF_COOKIE_NAME',
        defaultValue: 'arc_csrf_token',
      ),
      csrfHeaderName: String.fromEnvironment(
        'ARC_AUTH_CSRF_HEADER_NAME',
        defaultValue: 'X-CSRF-Token',
      ),
    );
  }

  Uri get apiBaseUri => Uri.parse(apiBaseUrl);

  Uri get wsUri {
    final uri = apiBaseUri;
    final wsScheme = uri.scheme == 'https' ? 'wss' : 'ws';
    return uri.replace(scheme: wsScheme, path: '/ws');
  }
}
