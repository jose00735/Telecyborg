# kotlinx.serialization: keep generated serializers for our data classes
-keepattributes *Annotation*, InnerClasses
-keep,includedescriptorclasses class com.telecyborg.**$$serializer { *; }
-keepclassmembers class com.telecyborg.** { *** Companion; }
-keepclasseswithmembers class com.telecyborg.** { kotlinx.serialization.KSerializer serializer(...); }
