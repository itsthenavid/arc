import 'csrf_cookie_reader_stub.dart'
    if (dart.library.html) 'csrf_cookie_reader_web.dart';

abstract class CsrfCookieReader {
  String? readCookie(String name);
}

CsrfCookieReader createCsrfCookieReader() => createCsrfCookieReaderImpl();
