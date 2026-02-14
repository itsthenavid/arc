// ignore_for_file: avoid_web_libraries_in_flutter, deprecated_member_use

import 'dart:html' as html;

import 'csrf_cookie_reader.dart';

class _WebCsrfCookieReader implements CsrfCookieReader {
  const _WebCsrfCookieReader();

  @override
  String? readCookie(String name) {
    final raw = html.document.cookie;
    if (raw == null || raw.isEmpty) {
      return null;
    }

    final parts = raw.split(';');
    for (final part in parts) {
      final trimmed = part.trim();
      if (!trimmed.startsWith('$name=')) {
        continue;
      }
      final value = trimmed.substring(name.length + 1);
      if (value.isEmpty) {
        return null;
      }
      return Uri.decodeComponent(value);
    }
    return null;
  }
}

CsrfCookieReader createCsrfCookieReaderImpl() => const _WebCsrfCookieReader();
