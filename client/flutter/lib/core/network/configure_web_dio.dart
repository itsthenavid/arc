import 'package:dio/dio.dart';

import 'configure_web_dio_stub.dart'
    if (dart.library.html) 'configure_web_dio_web.dart';

void configureWebDio(Dio dio) => configureWebDioImpl(dio);
