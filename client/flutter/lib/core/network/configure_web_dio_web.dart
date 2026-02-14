import 'package:dio/browser.dart';
import 'package:dio/dio.dart';

void configureWebDioImpl(Dio dio) {
  dio.httpClientAdapter = BrowserHttpClientAdapter(withCredentials: true);
}
