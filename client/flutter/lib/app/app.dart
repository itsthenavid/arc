import 'package:arc/app/theme/arc_theme.dart';
import 'package:arc/features/auth/presentation/auth_flow_page.dart';
import 'package:flutter/material.dart';

class ArcApp extends StatelessWidget {
  const ArcApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Arc Messenger',
      debugShowCheckedModeBanner: false,
      themeMode: ThemeMode.system,
      themeAnimationDuration: const Duration(milliseconds: 260),
      themeAnimationCurve: Curves.easeOutCubic,
      theme: ArcTheme.light(),
      darkTheme: ArcTheme.dark(),
      scrollBehavior: const MaterialScrollBehavior().copyWith(
        scrollbars: false,
      ),
      home: const AuthFlowPage(),
    );
  }
}
