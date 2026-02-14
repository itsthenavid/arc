import 'csrf_cookie_reader.dart';

class _NoopCsrfCookieReader implements CsrfCookieReader {
  const _NoopCsrfCookieReader();

  @override
  String? readCookie(String name) => null;
}

CsrfCookieReader createCsrfCookieReaderImpl() => const _NoopCsrfCookieReader();
