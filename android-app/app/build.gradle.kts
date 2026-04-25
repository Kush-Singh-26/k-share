plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "com.kshare.android"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.kshare.android"
        minSdk = 24
        targetSdk = 35
        versionCode = 1
        versionName = "6.0.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildTypes {
        signingConfigs {
            create("externalOverride") {
                storeFile = file("../release-key.jks")
                storePassword = "kush26k-share"
                keyAlias = "kshare-release"
                keyPassword = "kush26k-share"
            }
        }
        release {
            signingConfig = signingConfigs.getByName("externalOverride")
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }

    buildFeatures {
        compose = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.gson)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation("androidx.activity:activity-ktx:1.8.0")
    
    // Compose
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.activity.compose)
    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)

    // Networking
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("androidx.documentfile:documentfile:1.0.1")

    // Coroutines
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.7.3")

    // WorkManager (background discovery)
    implementation("androidx.work:work-runtime-ktx:2.9.0")

    testImplementation(libs.junit)
}
